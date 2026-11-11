.PHONY: test verify integration benchmark test-sliding-window

test:
	go test -race ./...

verify: test
	go vet ./...
	go build ./cmd/ratelimitd
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# Both variables are mandatory here: integrations must never silently skip.
integration:
	@test -n "$(REDIS_TEST_URL)" || (echo 'Set REDIS_TEST_URL to an isolated test Redis'; exit 1)
	@test -n "$(REDIS_SERVER_BIN)" || (echo 'Set REDIS_SERVER_BIN to a redis-server binary'; exit 1)
	go test -race -count=1 ./...

benchmark:
	go test -run '^$$' -bench . -benchmem ./tests/algorithm/fixedwindow

# Starts and cleans up a temporary Docker Redis unless REDIS_TEST_URL is supplied.
test-sliding-window:
	./tests/slidingwindow/run.sh

# Uses a disposable Docker Redis; LEAKY_LATENCY=1 also writes latency percentiles.
.PHONY: test-leaky-bucket
test-leaky-bucket:
	./tests/leakybucket/run.sh

# Starts and cleans up temporary Redis unless REDIS_TEST_URL is supplied.
.PHONY: test-sliding-counter
test-sliding-counter:
	./tests/slidingcounter/run.sh

.PHONY: test-token-bucket
test-token-bucket:
	./tests/tokenbucket/run.sh

# Measure all algorithms, then export the validated report for the algorithms page.
.PHONY: benchmark-latency
benchmark-latency:
	./tests/latency/run.sh

# Build once, then exercise the runtime image in isolated RAM/Redis Compose stacks.
.PHONY: test-docker
test-docker:
	docker build --target runtime -t rate-limiter:ci .
	RATE_LIMIT_IMAGE=rate-limiter:ci python3 tests/docker/smoke.py
