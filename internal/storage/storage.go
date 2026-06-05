package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type ModuleVersion struct {
	ModulePath         string
	Version            string
	FirstSeenAt        time.Time
	SourceTimestampRaw string
	LastCheckedAt      time.Time
}

func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) UpsertVersion(ctx context.Context, version ModuleVersion) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO module_versions (
  module_path, version, first_seen_at, source_timestamp_raw, last_checked_at
) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(module_path, version) DO UPDATE SET
  first_seen_at = excluded.first_seen_at,
  source_timestamp_raw = excluded.source_timestamp_raw,
  last_checked_at = excluded.last_checked_at
`, version.ModulePath, version.Version, formatTime(version.FirstSeenAt), version.SourceTimestampRaw, formatTime(version.LastCheckedAt))
	if err != nil {
		return fmt.Errorf("upsert module version: %w", err)
	}
	return nil
}

func (s *Store) UpsertVersions(ctx context.Context, versions []ModuleVersion) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert versions: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO module_versions (
  module_path, version, first_seen_at, source_timestamp_raw, last_checked_at
) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(module_path, version) DO UPDATE SET
  first_seen_at = excluded.first_seen_at,
  source_timestamp_raw = excluded.source_timestamp_raw,
  last_checked_at = excluded.last_checked_at
`)
	if err != nil {
		return fmt.Errorf("prepare upsert versions: %w", err)
	}
	defer stmt.Close()

	for _, version := range versions {
		if _, err := stmt.ExecContext(ctx, version.ModulePath, version.Version, formatTime(version.FirstSeenAt), version.SourceTimestampRaw, formatTime(version.LastCheckedAt)); err != nil {
			return fmt.Errorf("upsert %s@%s: %w", version.ModulePath, version.Version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert versions: %w", err)
	}
	return nil
}

func (s *Store) GetVersion(ctx context.Context, modulePath, version string) (ModuleVersion, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT module_path, version, first_seen_at, source_timestamp_raw, last_checked_at
FROM module_versions
WHERE module_path = ? AND version = ?
`, modulePath, version)
	got, err := scanModuleVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ModuleVersion{}, false, nil
	}
	if err != nil {
		return ModuleVersion{}, false, err
	}
	return got, true, nil
}

func (s *Store) ListModuleVersions(ctx context.Context, modulePath string) ([]ModuleVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT module_path, version, first_seen_at, source_timestamp_raw, last_checked_at
FROM module_versions
WHERE module_path = ?
`, modulePath)
	if err != nil {
		return nil, fmt.Errorf("list module versions: %w", err)
	}
	defer rows.Close()

	var versions []ModuleVersion
	for rows.Next() {
		version, err := scanModuleVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate module versions: %w", err)
	}
	return versions, nil
}

func (s *Store) SetCursor(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO cursor_state (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value
`, key, value)
	if err != nil {
		return fmt.Errorf("set cursor %q: %w", key, err)
	}
	return nil
}

func (s *Store) GetCursor(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM cursor_state WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get cursor %q: %w", key, err)
	}
	return value, true, nil
}

func (s *Store) CountKnownVersions(ctx context.Context) (int64, error) {
	var count int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM module_versions`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count known versions: %w", err)
	}
	return count, nil
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS module_versions (
  module_path TEXT NOT NULL,
  version TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  source_timestamp_raw TEXT NOT NULL,
  last_checked_at TEXT NOT NULL,
  PRIMARY KEY (module_path, version)
)`,
		`CREATE INDEX IF NOT EXISTS idx_module_versions_module ON module_versions(module_path)`,
		`CREATE INDEX IF NOT EXISTS idx_module_versions_first_seen ON module_versions(first_seen_at)`,
		`CREATE TABLE IF NOT EXISTS cursor_state (
  key TEXT NOT NULL PRIMARY KEY,
  value TEXT NOT NULL
)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanModuleVersion(row scanner) (ModuleVersion, error) {
	var version ModuleVersion
	var firstSeenRaw, lastCheckedRaw string
	if err := row.Scan(&version.ModulePath, &version.Version, &firstSeenRaw, &version.SourceTimestampRaw, &lastCheckedRaw); err != nil {
		return ModuleVersion{}, err
	}

	firstSeen, err := time.Parse(time.RFC3339Nano, firstSeenRaw)
	if err != nil {
		return ModuleVersion{}, fmt.Errorf("parse first_seen_at %q: %w", firstSeenRaw, err)
	}
	lastChecked, err := time.Parse(time.RFC3339Nano, lastCheckedRaw)
	if err != nil {
		return ModuleVersion{}, fmt.Errorf("parse last_checked_at %q: %w", lastCheckedRaw, err)
	}
	version.FirstSeenAt = firstSeen
	version.LastCheckedAt = lastChecked
	return version, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
