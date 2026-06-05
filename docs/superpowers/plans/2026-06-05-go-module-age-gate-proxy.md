# Go Module Age-Gate Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a minimal production-usable Go GOPROXY-compatible HTTP service that blocks module versions whose `index.golang.org` first-seen timestamp is younger than the configured age gate.

**Architecture:** The service is a thin policy facade in front of `proxy.golang.org`. SQLite stores module/version first-seen metadata and the index cursor; HTTP handlers filter or redirect based on local policy decisions.

**Tech Stack:** Go 1.26, `net/http`, `database/sql`, `modernc.org/sqlite`, `golang.org/x/mod/semver`, structured `log/slog`.

---

### Task 1: Core Policy, Semver, And Storage

**Files:**
- Create: `go.mod`
- Create: `internal/policy/policy_test.go`
- Create: `internal/policy/policy.go`
- Create: `internal/semverutil/semverutil_test.go`
- Create: `internal/semverutil/semverutil.go`
- Create: `internal/storage/storage_test.go`
- Create: `internal/storage/storage.go`

- [ ] **Step 1: Write failing tests**

Run: `go test ./internal/policy ./internal/semverutil ./internal/storage`

Expected: FAIL because policy parsing, latest selection, and SQLite store APIs do not exist yet.

- [ ] **Step 2: Implement minimal code**

Implement `/age/<duration>` parsing, default policy fallback, `unknown == block`, Go semver max selection, SQLite schema creation, version upsert/lookup, and cursor persistence.

- [ ] **Step 3: Verify**

Run: `go test ./internal/policy ./internal/semverutil ./internal/storage`

Expected: PASS.

### Task 2: Index Ingester

**Files:**
- Create: `internal/indexsync/indexsync_test.go`
- Create: `internal/indexsync/indexsync.go`

- [ ] **Step 1: Write failing fake-index test**

Run: `go test ./internal/indexsync`

Expected: FAIL because one-shot index sync is not implemented.

- [ ] **Step 2: Implement minimal sync**

Fetch `INDEX_BASE/index?since=<cursor>`, decode JSON lines with `Path`, `Version`, and `Timestamp`, upsert rows, and persist the max timestamp cursor.

- [ ] **Step 3: Verify**

Run: `go test ./internal/indexsync`

Expected: PASS.

### Task 3: HTTP GOPROXY Facade

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/upstream/upstream.go`
- Create: `internal/metrics/metrics.go`
- Create: `internal/httpapi/server_test.go`
- Create: `internal/httpapi/server.go`
- Create: `cmd/seasond/main.go`

- [ ] **Step 1: Write failing integration tests**

Run: `go test ./internal/httpapi`

Expected: FAIL because GOPROXY handlers are missing.

- [ ] **Step 2: Implement endpoints**

Implement `/healthz`, `/readyz`, `/metrics`, `/status`, `@v/list`, `@latest`, and direct `.info`, `.mod`, `.zip` paths. Filter lists, return filtered `@latest` JSON, redirect allowed artifacts, and return `403` for blocked or unknown versions.

- [ ] **Step 3: Verify**

Run: `go test ./internal/httpapi`

Expected: PASS.

### Task 4: Whole-Service Verification

**Files:**
- Modify as needed: files from Tasks 1-3.

- [ ] **Step 1: Format and test**

Run: `gofmt -w cmd internal` and `go test ./...`

Expected: PASS.

- [ ] **Step 2: Build binary**

Run: `go build ./cmd/seasond`

Expected: PASS.

- [ ] **Step 3: Go-tool smoke**

Run the local server against seeded metadata and use `GOPROXY=http://127.0.0.1:<port>/age/336h go mod download` in a temporary module.

Expected: allowed old versions download via redirects; fresh versions are blocked with `403`.
