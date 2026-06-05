//go:build e2e

package e2e

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/iamwavecut/seasond/internal/httpapi"
	"github.com/iamwavecut/seasond/internal/storage"
)

func TestGoModDownloadAllowsSeededOldVersion(t *testing.T) {
	server, store := startProxy(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	defer server.Close()
	defer store.Close()

	output, err := runGoModDownload(t, server.URL+"/age/336h")
	if err != nil {
		t.Fatalf("go mod download failed: %v\n%s", err, output)
	}
}

func TestGoModDownloadFailsClosedForSeededFreshVersion(t *testing.T) {
	server, store := startProxy(t, time.Now().UTC())
	defer server.Close()
	defer store.Close()

	output, err := runGoModDownload(t, server.URL+"/age/336h")
	if err == nil {
		t.Fatalf("go mod download unexpectedly succeeded:\n%s", output)
	}
}

func startProxy(t *testing.T, firstSeen time.Time) (*httptest.Server, *storage.Store) {
	t.Helper()

	store, err := storage.Open(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := store.UpsertVersion(t.Context(), storage.ModuleVersion{
		ModulePath:         "github.com/pkg/errors",
		Version:            "v0.9.1",
		FirstSeenAt:        firstSeen,
		SourceTimestampRaw: firstSeen.Format(time.RFC3339Nano),
		LastCheckedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertVersion returned error: %v", err)
	}

	handler := httpapi.NewHandler(httpapi.Config{
		Store:         store,
		UpstreamBase:  "https://proxy.golang.org",
		DefaultMinAge: 14 * 24 * time.Hour,
		HTTPClient:    &http.Client{Timeout: 20 * time.Second},
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return httptest.NewServer(handler), store
}

func runGoModDownload(t *testing.T, proxy string) (string, error) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module e2esmoke\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cmd := exec.CommandContext(t.Context(), "go", "mod", "download", "github.com/pkg/errors@v0.9.1")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOPROXY="+proxy,
		"GOSUMDB=off",
		"GONOPROXY=",
		"GONOSUMDB=",
		"GOPRIVATE=",
		"GOMODCACHE="+filepath.Join(dir, "modcache"),
		"GOCACHE="+filepath.Join(dir, "buildcache"),
	)
	output, err := cmd.CombinedOutput()
	makeWritable(filepath.Join(dir, "modcache"))
	return string(output), err
}

func makeWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(path, 0o755)
			return nil
		}
		_ = os.Chmod(path, 0o644)
		return nil
	})
}
