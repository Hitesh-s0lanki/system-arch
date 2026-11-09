.PHONY: test verify integration benchmark

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
