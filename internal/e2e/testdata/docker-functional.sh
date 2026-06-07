#!/bin/sh
set -eu

server_pid=""

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
}

fail() {
  echo "FAIL: $*" >&2
  if [ -f /tmp/seasond.log ]; then
    echo "--- seasond log ---" >&2
    tail -120 /tmp/seasond.log >&2 || true
  fi
  exit 1
}

trap cleanup EXIT INT TERM

apk add --no-cache ca-certificates curl >/dev/null

echo "container go: $(go version)"
selection="$(
  go run ./internal/e2e/testdata/indexpair \
    -lookback-days "${SEASOND_E2E_LOOKBACK_DAYS:-45}" \
    -max-pages "${SEASOND_E2E_MAX_INDEX_PAGES:-80}"
)"
printf '%s\n' "$selection"
eval "$selection"

go build -trimpath -o /tmp/seasond ./cmd/seasond

DB_PATH=/tmp/seasond-e2e.db \
LISTEN_ADDR=127.0.0.1:8080 \
BOOTSTRAP_SINCE="$BOOTSTRAP_SINCE" \
POLL_INTERVAL=1h \
HTTP_TIMEOUT=30s \
LOG_LEVEL=debug \
  /tmp/seasond >/tmp/seasond.log 2>&1 &
server_pid="$!"

for _ in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl -fsS http://127.0.0.1:8080/healthz >/dev/null || fail "seasond did not become healthy"

demo_dir=/tmp/seasond-demo
mkdir -p "$demo_dir"
cat >"$demo_dir/go.mod" <<'EOF'
module example.com/seasond-e2e

go 1.26
EOF

proxy="http://127.0.0.1:8080/age/${MIN_AGE}"
echo "proxy: $proxy"
echo "module: $MODULE_PATH"
echo "allowed: $ALLOWED_VERSION first_seen=$ALLOWED_TIMESTAMP"
echo "blocked: $BLOCKED_VERSION first_seen=$BLOCKED_TIMESTAMP"

go_env() {
  env \
    GOPROXY="$proxy" \
    GOSUMDB=off \
    GONOPROXY= \
    GONOSUMDB= \
    GOPRIVATE= \
    GOMODCACHE=/tmp/seasond-modcache \
    GOCACHE=/tmp/seasond-gocache \
    "$@"
}

allowed_probe=/tmp/seasond-allowed-probe.txt
attempt=0
until (
  cd "$demo_dir"
  go_env go list -m "$MODULE_PATH@$ALLOWED_VERSION" >"$allowed_probe" 2>&1
); do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 90 ]; then
    cat "$allowed_probe" >&2 || true
    fail "allowed version was not admitted after index sync"
  fi
  sleep 1
done

(
  cd "$demo_dir"
  go_env go mod download "$MODULE_PATH@$ALLOWED_VERSION"
) || fail "allowed go mod download failed"

set +e
blocked_output="$(
  cd "$demo_dir"
  go_env go mod download "$MODULE_PATH@$BLOCKED_VERSION" 2>&1
)"
blocked_status=$?
set -e
echo "$blocked_output"
if [ "$blocked_status" -eq 0 ]; then
  fail "blocked go mod download unexpectedly succeeded"
fi
case "$blocked_output" in
  *"403 Forbidden"* | *"too_fresh"*) ;;
  *) fail "blocked go mod download failed for an unexpected reason" ;;
esac

curl -fsS http://127.0.0.1:8080/status
echo
echo "docker functional e2e ok"
