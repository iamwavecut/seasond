package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/iamwavecut/seasond/internal/indexsync"
	"github.com/iamwavecut/seasond/internal/metrics"
	"github.com/iamwavecut/seasond/internal/policy"
	"github.com/iamwavecut/seasond/internal/semverutil"
	"github.com/iamwavecut/seasond/internal/storage"
	"github.com/iamwavecut/seasond/internal/upstream"
)

type Config struct {
	Store                 *storage.Store
	UpstreamBase          string
	DefaultMinAge         time.Duration
	HTTPClient            *http.Client
	Now                   func() time.Time
	Logger                *slog.Logger
	AllowRedirectsForInfo bool
}

type Server struct {
	store                 *storage.Store
	upstream              *upstream.Client
	defaultMinAge         time.Duration
	now                   func() time.Time
	logger                *slog.Logger
	metrics               *metrics.Registry
	allowRedirectsForInfo bool
}

type proxyRequest struct {
	endpoint  string
	module    string
	version   string
	extension string
}

func NewHandler(cfg Config) http.Handler {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		store:                 cfg.Store,
		upstream:              upstream.New(cfg.UpstreamBase, cfg.HTTPClient),
		defaultMinAge:         cfg.DefaultMinAge,
		now:                   now,
		logger:                logger,
		metrics:               metrics.New(),
		allowRedirectsForInfo: cfg.AllowRedirectsForInfo,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := s.now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	endpoint, decision := s.serve(rec, r)
	duration := s.now().Sub(start)
	s.metrics.ObserveRequest(endpoint, rec.status, decision, duration)
	s.logger.InfoContext(r.Context(), "request",
		"method", r.Method,
		"path", r.URL.Path,
		"endpoint", endpoint,
		"decision", decision,
		"duration_ms", duration.Milliseconds(),
		"status_code", rec.status,
	)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) (string, string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return "unknown", "error"
	}

	switch r.URL.Path {
	case "/healthz":
		_, _ = io.WriteString(w, "ok\n")
		return "healthz", "allow"
	case "/readyz":
		if err := s.store.Ping(r.Context()); err != nil {
			http.Error(w, "database not ready", http.StatusServiceUnavailable)
			return "readyz", "error"
		}
		_, _ = io.WriteString(w, "ok\n")
		return "readyz", "allow"
	case "/status":
		s.writeStatus(w, r)
		return "status", "allow"
	case "/metrics":
		s.writeMetrics(w, r)
		return "metrics", "allow"
	}

	pol, remaining, err := policy.ParsePrefix(r.URL.Path, s.defaultMinAge)
	if err != nil {
		http.Error(w, "invalid policy prefix", http.StatusBadRequest)
		return "proxy", "error"
	}
	req, err := parseProxyRequest(remaining)
	if err != nil {
		http.NotFound(w, r)
		return "proxy", "not_found"
	}

	switch req.endpoint {
	case "list":
		return req.endpoint, s.serveList(w, r, pol, req)
	case "latest":
		return req.endpoint, s.serveLatest(w, r, pol, req)
	case "info":
		return req.endpoint, s.serveInfo(w, r, pol, req)
	case "mod", "zip":
		return req.endpoint, s.serveArtifact(w, r, pol, req)
	default:
		http.NotFound(w, r)
		return "proxy", "not_found"
	}
}

func (s *Server) serveList(w http.ResponseWriter, r *http.Request, pol policy.Policy, req proxyRequest) string {
	versions, status, err := s.upstream.List(r.Context(), req.module)
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return "error"
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		http.Error(w, http.StatusText(status), status)
		return "not_found"
	}
	if status != http.StatusOK {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return "error"
	}

	allowed, err := s.allowedVersions(r.Context(), pol, req.module, versions)
	if err != nil {
		http.Error(w, "metadata error", http.StatusInternalServerError)
		return "error"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, version := range semverutil.Sort(allowed) {
		_, _ = fmt.Fprintln(w, version)
	}
	return "allow"
}

func (s *Server) serveLatest(w http.ResponseWriter, r *http.Request, pol policy.Policy, req proxyRequest) string {
	versions, status, err := s.upstream.List(r.Context(), req.module)
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return "error"
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		http.Error(w, http.StatusText(status), status)
		return "not_found"
	}
	if status != http.StatusOK {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return "error"
	}

	allowed, err := s.allowedVersions(r.Context(), pol, req.module, versions)
	if err != nil {
		http.Error(w, "metadata error", http.StatusInternalServerError)
		return "error"
	}
	latest, ok := semverutil.Latest(allowed)
	if !ok {
		http.Error(w, "no allowed version", http.StatusForbidden)
		return "block"
	}

	body, contentType, status, err := s.upstream.Info(r.Context(), req.module, latest)
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return "error"
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		http.Error(w, http.StatusText(status), status)
		return "not_found"
	}
	if status != http.StatusOK {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return "error"
	}
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
	return "allow"
}

func (s *Server) serveInfo(w http.ResponseWriter, r *http.Request, pol policy.Policy, req proxyRequest) string {
	decision, err := s.decideVersion(r.Context(), pol, req.module, req.version)
	if err != nil {
		http.Error(w, "metadata error", http.StatusInternalServerError)
		return "error"
	}
	if !decision.Allowed {
		http.Error(w, decision.Reason, http.StatusForbidden)
		return "block"
	}
	if s.allowRedirectsForInfo {
		http.Redirect(w, r, s.upstream.ArtifactURL(req.module, req.version, ".info"), http.StatusFound)
		return "allow"
	}

	body, contentType, status, err := s.upstream.Info(r.Context(), req.module, req.version)
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return "error"
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		http.Error(w, http.StatusText(status), status)
		return "not_found"
	}
	if status != http.StatusOK {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return "error"
	}
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
	return "allow"
}

func (s *Server) serveArtifact(w http.ResponseWriter, r *http.Request, pol policy.Policy, req proxyRequest) string {
	decision, err := s.decideVersion(r.Context(), pol, req.module, req.version)
	if err != nil {
		http.Error(w, "metadata error", http.StatusInternalServerError)
		return "error"
	}
	if !decision.Allowed {
		http.Error(w, decision.Reason, http.StatusForbidden)
		return "block"
	}
	http.Redirect(w, r, s.upstream.ArtifactURL(req.module, req.version, req.extension), http.StatusFound)
	return "allow"
}

func (s *Server) allowedVersions(ctx context.Context, pol policy.Policy, module string, versions []string) ([]string, error) {
	now := s.now()
	allowed := make([]string, 0, len(versions))
	for _, version := range semverutil.Sort(versions) {
		got, ok, err := s.store.GetVersion(ctx, module, version)
		if err != nil {
			return nil, err
		}
		var firstSeen *time.Time
		if ok {
			firstSeen = &got.FirstSeenAt
		}
		if pol.Decide(firstSeen, now).Allowed {
			allowed = append(allowed, version)
		}
	}
	return allowed, nil
}

func (s *Server) decideVersion(ctx context.Context, pol policy.Policy, module, version string) (policy.Decision, error) {
	got, ok, err := s.store.GetVersion(ctx, module, version)
	if err != nil {
		return policy.Decision{}, err
	}
	if !ok {
		return pol.Decide(nil, s.now()), nil
	}
	return pol.Decide(&got.FirstSeenAt, s.now()), nil
}

func (s *Server) writeStatus(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.CountKnownVersions(r.Context())
	if err != nil {
		http.Error(w, "metadata error", http.StatusInternalServerError)
		return
	}
	indexSince, _, _ := s.store.GetCursor(r.Context(), indexsync.CursorIndexSince)
	lastSync, _, _ := s.store.GetCursor(r.Context(), indexsync.CursorLastSuccessfulSync)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"known_versions":           count,
		"index_since":              indexSince,
		"last_successful_sync_at":  lastSync,
		"default_min_age_seconds":  s.defaultMinAge.Seconds(),
		"allow_redirects_for_info": s.allowRedirectsForInfo,
	})
}

func (s *Server) writeMetrics(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.CountKnownVersions(r.Context())
	if err != nil {
		http.Error(w, "metadata error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	s.metrics.WritePrometheus(w, count)
}

func parseProxyRequest(path string) (proxyRequest, error) {
	trimmed := strings.TrimPrefix(path, "/")
	if module, ok := strings.CutSuffix(trimmed, "/@v/list"); ok && module != "" {
		return proxyRequest{endpoint: "list", module: module}, nil
	}
	if module, ok := strings.CutSuffix(trimmed, "/@latest"); ok && module != "" {
		return proxyRequest{endpoint: "latest", module: module}, nil
	}
	module, artifact, ok := strings.Cut(trimmed, "/@v/")
	if !ok || module == "" || artifact == "" {
		return proxyRequest{}, fmt.Errorf("not a proxy path")
	}
	for _, ext := range []string{".info", ".mod", ".zip"} {
		if version, ok := strings.CutSuffix(artifact, ext); ok && version != "" {
			return proxyRequest{
				endpoint:  strings.TrimPrefix(ext, "."),
				module:    module,
				version:   version,
				extension: ext,
			}, nil
		}
	}
	return proxyRequest{}, fmt.Errorf("unsupported artifact")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
