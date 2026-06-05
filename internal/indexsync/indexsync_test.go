package indexsync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/iamwavecut/seasond/internal/storage"
)

func TestSyncOnceStoresIndexRowsAndAdvancesCursor(t *testing.T) {
	first := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	second := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)

	var seenSince string
	index := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index" {
			t.Fatalf("path = %q, want /index", r.URL.Path)
		}
		seenSince = r.URL.Query().Get("since")
		for _, row := range []IndexRow{
			{Path: "github.com/pkg/errors", Version: "v0.9.1", Timestamp: first},
			{Path: "golang.org/x/mod", Version: "v0.36.0", Timestamp: second},
		} {
			if err := json.NewEncoder(w).Encode(row); err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}
		}
	}))
	defer index.Close()

	store, err := storage.Open(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	syncer := New(store, Config{
		IndexBase:      index.URL,
		BootstrapSince: "2026-04-01T00:00:00Z",
		HTTPClient:     index.Client(),
		Now:            func() time.Time { return second.Add(time.Hour) },
	})

	stats, err := syncer.SyncOnce(t.Context())
	if err != nil {
		t.Fatalf("SyncOnce returned error: %v", err)
	}
	if seenSince != "2026-04-01T00:00:00Z" {
		t.Fatalf("since = %q, want bootstrap cursor", seenSince)
	}
	if stats.Rows != 2 {
		t.Fatalf("Rows = %d, want 2", stats.Rows)
	}

	got, ok, err := store.GetVersion(t.Context(), "golang.org/x/mod", "v0.36.0")
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}
	if !ok {
		t.Fatal("synced version not found")
	}
	if !got.FirstSeenAt.Equal(second) {
		t.Fatalf("FirstSeenAt = %s, want %s", got.FirstSeenAt, second)
	}

	cursor, ok, err := store.GetCursor(t.Context(), CursorIndexSince)
	if err != nil {
		t.Fatalf("GetCursor returned error: %v", err)
	}
	if !ok || cursor != second.Format(time.RFC3339Nano) {
		t.Fatalf("cursor = %q ok=%v, want second timestamp", cursor, ok)
	}
}
