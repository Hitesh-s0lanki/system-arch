#!/bin/sh
# Uses a dedicated temporary Redis by default. No Redis integration test can skip.
set -eu
cd "$(dirname "$0")/../.."
container_id=""
build_dir=$(mktemp -d)
cleanup() {
    rm -rf "$build_dir"
    if [ -n "$container_id" ]; then
        docker rm -f "$container_id" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if [ -z "${REDIS_TEST_URL:-}" ]; then
    container_id=$(docker run --rm -d -p 127.0.0.1::6379 "${TOKEN_BUCKET_REDIS_IMAGE:-redis:7.4-alpine}" redis-server --save '' --appendonly no)
    attempts=0
    until docker exec "$container_id" redis-cli ping >/dev/null 2>&1; do
        attempts=$((attempts + 1))
        if [ "$attempts" -ge 30 ]; then
            echo 'Temporary Redis did not become ready' >&2
            exit 1
        fi
        sleep 0.2
    done
    port=$(docker inspect --format '{{(index (index .NetworkSettings.Ports "6379/tcp") 0).HostPort}}' "$container_id")
    REDIS_TEST_URL="redis://127.0.0.1:$port/0"
    export REDIS_TEST_URL
fi
go build -o "$build_dir/ratelimitd" ./cmd/ratelimitd
TOKEN_BUCKET_TEST_BINARY="$build_dir/ratelimitd"
export TOKEN_BUCKET_TEST_BINARY
go test -race -count=1 -v ./tests/algorithm/tokenbucket/...
go test -race -count=1 -v -run TokenBucket ./tests/sdk ./tests/internal/proxyapp
