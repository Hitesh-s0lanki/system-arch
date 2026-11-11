#!/bin/sh
# Run RAM, real Redis, SDK, and HTTP proxy checks. Own and remove a temporary
# Redis container unless the caller supplies a dedicated REDIS_TEST_URL.
set -eu
cd "$(dirname "$0")/../.."
container_id=""
cleanup() {
    if [ -n "$container_id" ]; then
        docker rm -f "$container_id" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if [ -z "${REDIS_TEST_URL:-}" ]; then
    container_id=$(docker run --rm -d -p 127.0.0.1::6379 "${SLIDING_REDIS_IMAGE:-redis:7-alpine}" redis-server --save '' --appendonly no)
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
# A nonempty URL ensures the Redis cases cannot silently skip.
go test -race -shuffle=on -count="${SLIDING_TEST_COUNT:-1}" -timeout=2m -v ./tests/algorithm/slidingwindow/...
go test -race -shuffle=on -count="${SLIDING_TEST_COUNT:-1}" -timeout=2m -v \
    -run 'TestSlidingWindow|TestRejectUnknownOrInconsistentAlgorithm' \
    ./tests/sdk ./tests/internal/proxyapp
