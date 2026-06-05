package indexsync

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iamwavecut/seasond/internal/storage"
)

const (
	CursorIndexSince         = "index_since"
	CursorLastPollAt         = "last_poll_at"
	CursorLastSuccessfulSync = "last_successful_sync_at"
)

type Config struct {
	IndexBase      string
	BootstrapSince string
	HTTPClient     *http.Client
	Now            func() time.Time
}

type Syncer struct {
	store          *storage.Store
	indexBase      string
	bootstrapSince string
	client         *http.Client
	now            func() time.Time
}

type IndexRow struct {
	Path      string    `json:"Path"`
	Version   string    `json:"Version"`
	Timestamp time.Time `json:"Timestamp"`
}

type Stats struct {
	Rows int
}

func New(store *storage.Store, cfg Config) *Syncer {
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Syncer{
		store:          store,
		indexBase:      strings.TrimRight(cfg.IndexBase, "/"),
		bootstrapSince: cfg.BootstrapSince,
		client:         client,
		now:            now,
	}
}

func (s *Syncer) SyncOnce(ctx context.Context) (Stats, error) {
	pollAt := s.now().UTC()
	if err := s.store.SetCursor(ctx, CursorLastPollAt, pollAt.Format(time.RFC3339Nano)); err != nil {
		return Stats{}, err
	}

	since, ok, err := s.store.GetCursor(ctx, CursorIndexSince)
	if err != nil {
		return Stats{}, err
	}
	if !ok {
		since = s.bootstrapSince
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.indexURL(since), nil)
	if err != nil {
		return Stats{}, fmt.Errorf("create index request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Stats{}, fmt.Errorf("fetch index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Stats{}, fmt.Errorf("fetch index: status %d", resp.StatusCode)
	}

	var versions []storage.ModuleVersion
	var maxTimestamp time.Time
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row IndexRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return Stats{}, fmt.Errorf("decode index row: %w", err)
		}
		raw := row.Timestamp.UTC().Format(time.RFC3339Nano)
		versions = append(versions, storage.ModuleVersion{
			ModulePath:         row.Path,
			Version:            row.Version,
			FirstSeenAt:        row.Timestamp.UTC(),
			SourceTimestampRaw: raw,
			LastCheckedAt:      pollAt,
		})
		if row.Timestamp.After(maxTimestamp) {
			maxTimestamp = row.Timestamp.UTC()
		}
	}
	if err := scanner.Err(); err != nil {
		return Stats{}, fmt.Errorf("read index response: %w", err)
	}

	if err := s.store.UpsertVersions(ctx, versions); err != nil {
		return Stats{}, err
	}
	if !maxTimestamp.IsZero() {
		if err := s.store.SetCursor(ctx, CursorIndexSince, maxTimestamp.Format(time.RFC3339Nano)); err != nil {
			return Stats{}, err
		}
	}
	if err := s.store.SetCursor(ctx, CursorLastSuccessfulSync, pollAt.Format(time.RFC3339Nano)); err != nil {
		return Stats{}, err
	}
	return Stats{Rows: len(versions)}, nil
}

func (s *Syncer) indexURL(since string) string {
	values := url.Values{}
	if since != "" {
		values.Set("since", since)
	}
	if encoded := values.Encode(); encoded != "" {
		return s.indexBase + "/index?" + encoded
	}
	return s.indexBase + "/index"
}
