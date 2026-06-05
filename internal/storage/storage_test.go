package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsVersionsAndCursor(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "metadata.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	firstSeen := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := store.UpsertVersion(t.Context(), ModuleVersion{
		ModulePath:         "github.com/pkg/errors",
		Version:            "v0.9.1",
		FirstSeenAt:        firstSeen,
		SourceTimestampRaw: firstSeen.Format(time.RFC3339Nano),
		LastCheckedAt:      firstSeen.Add(time.Hour),
	}); err != nil {
		t.Fatalf("UpsertVersion returned error: %v", err)
	}
	if err := store.SetCursor(t.Context(), "index_since", firstSeen.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetCursor returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen returned error: %v", err)
	}
	defer reopened.Close()

	got, ok, err := reopened.GetVersion(t.Context(), "github.com/pkg/errors", "v0.9.1")
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetVersion ok=false, want persisted row")
	}
	if !got.FirstSeenAt.Equal(firstSeen) {
		t.Fatalf("FirstSeenAt = %s, want %s", got.FirstSeenAt, firstSeen)
	}

	cursor, ok, err := reopened.GetCursor(t.Context(), "index_since")
	if err != nil {
		t.Fatalf("GetCursor returned error: %v", err)
	}
	if !ok || cursor != firstSeen.Format(time.RFC3339Nano) {
		t.Fatalf("cursor = %q ok=%v, want persisted timestamp", cursor, ok)
	}

	count, err := reopened.CountKnownVersions(t.Context())
	if err != nil {
		t.Fatalf("CountKnownVersions returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountKnownVersions = %d, want 1", count)
	}
}
