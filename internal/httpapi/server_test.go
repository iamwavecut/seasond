package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamwavecut/seasond/internal/storage"
)

func TestListFiltersFreshVersions(t *testing.T) {
	fixture := newHTTPFixture(t)
	defer fixture.close()

	resp, body := fixture.get("/age/336h/github.com/acme/widget/@v/list")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%q, want 200", resp.StatusCode, body)
	}
	if body != "v1.0.0\n" {
		t.Fatalf("body = %q, want only old version", body)
	}
}

func TestLatestReturnsNewestAllowedInfo(t *testing.T) {
	fixture := newHTTPFixture(t)
	defer fixture.close()

	resp, body := fixture.get("/age/336h/github.com/acme/widget/@latest")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%q, want 200", resp.StatusCode, body)
	}

	var info struct {
		Version string
		Time    time.Time
	}
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatalf("Unmarshal returned error: %v; body=%q", err, body)
	}
	if info.Version != "v1.0.0" {
		t.Fatalf("Version = %q, want latest allowed version", info.Version)
	}
}

func TestArtifactsRedirectOnlyWhenVersionIsAllowed(t *testing.T) {
	fixture := newHTTPFixture(t)
	defer fixture.close()

	resp, body := fixture.get("/age/336h/github.com/acme/widget/@v/v1.0.0.mod")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("allowed mod status = %d body=%q, want 302", resp.StatusCode, body)
	}
	wantLocation := fixture.upstream.URL + "/github.com/acme/widget/@v/v1.0.0.mod"
	if got := resp.Header.Get("Location"); got != wantLocation {
		t.Fatalf("Location = %q, want %q", got, wantLocation)
	}

	resp, body = fixture.get("/age/336h/github.com/acme/widget/@v/v1.1.0.zip")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("fresh zip status = %d body=%q, want 403", resp.StatusCode, body)
	}

	resp, body = fixture.get("/age/336h/github.com/acme/widget/@v/v9.9.9.info")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unknown info status = %d body=%q, want 403 fail-closed", resp.StatusCode, body)
	}
}

func TestHealthReadyStatusAndMetrics(t *testing.T) {
	fixture := newHTTPFixture(t)
	defer fixture.close()

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, body := fixture.get(path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d body=%q, want 200", path, resp.StatusCode, body)
		}
	}

	resp, body := fixture.get("/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint status = %d body=%q, want 200", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"known_versions":2`) {
		t.Fatalf("status body = %q, want known_versions count", body)
	}

	resp, body = fixture.get("/metrics")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d body=%q, want 200", resp.StatusCode, body)
	}
	if !strings.Contains(body, "seasond_known_versions_total 2") {
		t.Fatalf("metrics body = %q, want known versions metric", body)
	}
}

type httpFixture struct {
	store    *storage.Store
	upstream *httptest.Server
	server   *httptest.Server
	client   *http.Client
}

func newHTTPFixture(t *testing.T) *httpFixture {
	t.Helper()

	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour)
	fresh := now.Add(-2 * 24 * time.Hour)

	store, err := storage.Open(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	for _, row := range []storage.ModuleVersion{
		{
			ModulePath:         "github.com/acme/widget",
			Version:            "v1.0.0",
			FirstSeenAt:        old,
			SourceTimestampRaw: old.Format(time.RFC3339Nano),
			LastCheckedAt:      now,
		},
		{
			ModulePath:         "github.com/acme/widget",
			Version:            "v1.1.0",
			FirstSeenAt:        fresh,
			SourceTimestampRaw: fresh.Format(time.RFC3339Nano),
			LastCheckedAt:      now,
		},
	} {
		if err := store.UpsertVersion(t.Context(), row); err != nil {
			t.Fatalf("UpsertVersion returned error: %v", err)
		}
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/acme/widget/@v/list":
			_, _ = io.WriteString(w, "v1.0.0\nv1.1.0\n")
		case "/github.com/acme/widget/@v/v1.0.0.info":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"Version":"v1.0.0","Time":"2026-05-01T00:00:00Z"}`)
		case "/github.com/acme/widget/@v/v1.1.0.info":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"Version":"v1.1.0","Time":"2026-06-01T00:00:00Z"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	handler := NewHandler(Config{
		Store:         store,
		UpstreamBase:  upstream.URL,
		DefaultMinAge: 14 * 24 * time.Hour,
		HTTPClient:    upstream.Client(),
		Now:           func() time.Time { return now },
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	server := httptest.NewServer(handler)

	return &httpFixture{
		store:    store,
		upstream: upstream,
		server:   server,
		client: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (f *httpFixture) close() {
	f.server.Close()
	f.upstream.Close()
	_ = f.store.Close()
}

func (f *httpFixture) get(path string) (*http.Response, string) {
	resp, err := f.client.Get(f.server.URL + path)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return resp, string(body)
}
