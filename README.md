# seasond

[![CI](https://github.com/iamwavecut/seasond/actions/workflows/ci.yml/badge.svg)](https://github.com/iamwavecut/seasond/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iamwavecut/seasond.svg)](https://pkg.go.dev/github.com/iamwavecut/seasond)
[![Go Report Card](https://goreportcard.com/badge/github.com/iamwavecut/seasond)](https://goreportcard.com/report/github.com/iamwavecut/seasond)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`seasond` is a small GOPROXY-compatible HTTP service that blocks Go module
versions until they have been visible in the public Go module ecosystem for a
configured amount of time.

The default policy is a 14-day quarantine. Fresh or unknown versions fail
closed with `403 Forbidden`, while allowed `.mod` and `.zip` artifacts are
served by redirecting the Go tool to `proxy.golang.org`.

## Why

Most dependency updates are boring. The dangerous ones are often very new:
compromised maintainer accounts, poisoned tags, fast rollback incidents, or
fresh supply-chain attacks that need time to be noticed.

`seasond` adds a deliberately simple age gate in front of the public Go module
proxy:

- use `index.golang.org` as the first-seen timestamp source;
- keep module/version metadata in local SQLite;
- filter `@latest` and `@v/list` consistently;
- block direct requests for fresh or unknown versions;
- avoid proxying large module bodies through the service.

It is a policy layer, not a mirror.

## Status

This is an MVP for public Go modules. It is useful for experiments, local
policy enforcement, and small deployments, but it intentionally does not handle
private modules, custom VCS fallback, checksum database replacement, allowlists,
denylists, or a UI.

## Install

Requires Go 1.26 or newer.

```bash
go install github.com/iamwavecut/seasond/cmd/modguard@latest
```

Or build from a checkout:

```bash
go build -o modguard ./cmd/modguard
```

## Quick Start

Start the service with a persistent SQLite database:

```bash
DB_PATH=./seasond.db \
BOOTSTRAP_SINCE=2026-01-01T00:00:00Z \
LISTEN_ADDR=:8080 \
modguard
```

Use it as a Go proxy:

```bash
GOPROXY=http://localhost:8080/age/14d \
go list -m -versions github.com/pkg/errors
```

Download through the default 14-day policy:

```bash
GOPROXY=http://localhost:8080/age/336h \
go mod download github.com/pkg/errors@v0.9.1
```

For strict enforcement, do not append `,direct` or another proxy to `GOPROXY`.
Blocked versions return `403`, which the Go command does not treat as a normal
fallback signal; keeping `GOPROXY` single-hop makes the policy boundary easier
to reason about.

## How It Works

`seasond` has three main pieces:

1. **HTTP facade**: accepts a subset of the GOPROXY protocol used by the Go
   command.
2. **SQLite metadata store**: records `(module_path, version, first_seen_at)`
   plus index sync cursors.
3. **Index ingester**: polls `index.golang.org/index?since=...` and upserts
   first-seen timestamps.

A version is allowed when:

```text
now - first_seen_at >= min_age
```

If a version is missing from the local metadata database, it is blocked. This is
intentional: unknown means "not yet safe to serve".

## Supported Proxy Paths

The service supports both a root default policy and path-based age policies.

```text
http://localhost:8080/github.com/pkg/errors/@v/list
http://localhost:8080/age/14d/github.com/pkg/errors/@v/list
http://localhost:8080/age/336h/github.com/pkg/errors/@latest
http://localhost:8080/age/14d/github.com/pkg/errors/@v/v0.9.1.info
http://localhost:8080/age/14d/github.com/pkg/errors/@v/v0.9.1.mod
http://localhost:8080/age/14d/github.com/pkg/errors/@v/v0.9.1.zip
```

Path age values accept `d` for days and Go duration strings such as `336h`.

## Request Behavior

| Path | Allowed version | Fresh or unknown version | Upstream missing |
| --- | --- | --- | --- |
| `@v/list` | `200 OK` with filtered versions | omitted from list | `404` or `410` |
| `@latest` | `200 OK` with selected `.info` JSON | `403 Forbidden` if nothing is allowed | `404` or `410` |
| `.info` | `200 OK` by default | `403 Forbidden` | `404` or `410` |
| `.mod` | `302 Found` to upstream | `403 Forbidden` | not checked before redirect |
| `.zip` | `302 Found` to upstream | `403 Forbidden` | not checked before redirect |

Set `ALLOW_REDIRECTS_FOR_INFO=true` if you also want allowed `.info` requests
to redirect instead of being fetched and returned by the service.

## Configuration

All configuration is environment-based.

| Variable | Default | Description |
| --- | --- | --- |
| `LISTEN_ADDR` | `:8080` | HTTP listen address. |
| `UPSTREAM_PROXY_BASE` | `https://proxy.golang.org` | Public module proxy used for lists, info, and artifact redirects. |
| `INDEX_BASE` | `https://index.golang.org` | Go module index base URL. |
| `DEFAULT_MIN_AGE` | `336h` | Root policy age as a Go duration string. |
| `DB_PATH` | `modguard.db` | SQLite database path. |
| `POLL_INTERVAL` | `60s` | Delay between index sync attempts. |
| `BOOTSTRAP_SINCE` | empty | Initial index cursor when no cursor exists in SQLite. |
| `HTTP_TIMEOUT` | `15s` | Timeout for upstream/index HTTP calls and graceful shutdown. |
| `ALLOW_REDIRECTS_FOR_INFO` | `false` | Redirect allowed `.info` requests instead of returning JSON. |
| `USE_CACHED_ONLY_UPSTREAM` | `false` | Append `/cached-only` to the upstream proxy base. |
| `LOG_LEVEL` | `info` | One of `debug`, `info`, `warn`, or `error`. |

### Bootstrap Notes

The service only knows about versions that have been ingested into SQLite.
Choose `BOOTSTRAP_SINCE` far enough back for your use case and let the ingester
catch up. The cursor is persisted as `index_since`, so restarts continue from
the last successful sync instead of starting over.

## Health and Observability

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/status
curl http://localhost:8080/metrics
```

Endpoints:

- `/healthz`: process liveness.
- `/readyz`: SQLite connectivity.
- `/status`: JSON summary with known version count and sync cursors.
- `/metrics`: Prometheus-style text metrics.

Metric names currently emitted:

- `modguard_requests_total`
- `modguard_request_duration_seconds_sum`
- `modguard_blocked_requests_total`
- `modguard_known_versions_total`

Request logs are structured with `method`, `path`, `endpoint`, `decision`,
`duration_ms`, and `status_code`.

## Development

Run the standard checks:

```bash
go test ./...
go vet ./...
go build -o /tmp/seasond-modguard ./cmd/modguard
```

Run the networked end-to-end smoke test with the real Go command:

```bash
go test -tags=e2e ./internal/e2e
```

The e2e test starts a local proxy handler, seeds SQLite metadata, and verifies
that `go mod download` succeeds for an old version and fails closed for a fresh
version.

## Security Model

`seasond` helps reduce exposure to very fresh public module releases. It does
not replace Go checksum verification and does not prove that an older version is
safe. It only enforces a first-seen age policy based on public Go module index
timestamps.

Recommended strict mode:

```bash
GOPROXY=https://your-seasond.example.com/age/14d
```

Avoid:

```bash
GOPROXY=https://your-seasond.example.com/age/14d,direct
```

The second form allows fallback paths outside the policy boundary.

## Project Layout

```text
cmd/modguard          binary entrypoint
internal/config       environment configuration
internal/httpapi      GOPROXY-compatible HTTP facade
internal/indexsync    index.golang.org ingestion
internal/metrics      minimal Prometheus-style metrics
internal/policy       age-gate policy parsing and decisions
internal/semverutil   Go module semver sorting helpers
internal/storage      SQLite metadata store
internal/upstream     proxy.golang.org client
internal/e2e          opt-in Go tool smoke tests
```

## License

MIT. See [LICENSE](LICENSE).
