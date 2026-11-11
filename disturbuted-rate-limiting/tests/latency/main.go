// Command latency compares all implemented algorithms against RAM and real Redis.
// Run without -race against a disposable local Redis; no external upstream is used.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	fw "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	rf "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	lb "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket"
	rl "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket/redis"
	sc "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingcounter"
	rc "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingcounter/redis"
	sw "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingwindow"
	rs "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/slidingwindow/redis"
	tb "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket"
	rt "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/tokenbucket/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/proxyapp"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	redis "github.com/redis/go-redis/v9"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	Algorithm  string  `json:"algorithm"`
	Backend    string  `json:"backend"`
	Scenario   string  `json:"scenario"`
	Workers    int     `json:"workers"`
	Samples    int     `json:"samples"`
	Allowed    int64   `json:"allowed"`
	Rejected   int64   `json:"rejected"`
	Errors     int64   `json:"errors"`
	P50        float64 `json:"p50_us"`
	P95        float64 `json:"p95_us"`
	P99        float64 `json:"p99_us"`
	Max        float64 `json:"max_us"`
	Throughput float64 `json:"operations_per_second"`
}
type evaluator interface {
	Evaluate(context.Context, string) (ratelimit.Decision, error)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func measure(algorithm, backend, scenario string, workers, n int, call func(int) (bool, error)) result {
	// Warm connections, scripts, map entries and HTTP transports before timing.
	for i := range 100 {
		_, err := call(i)
		must(err)
	}
	samples := make([]time.Duration, n)
	var next, allowed, rejected, failures atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				i := int(next.Add(1) - 1)
				if i >= n {
					return
				}
				before := time.Now()
				ok, err := call(i)
				samples[i] = time.Since(before)
				if err != nil {
					failures.Add(1)
				} else if ok {
					allowed.Add(1)
				} else {
					rejected.Add(1)
				}
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	percentile := func(p float64) float64 {
		return float64(samples[max(0, int(math.Ceil(p*float64(n)))-1)]) / float64(time.Microsecond)
	}
	return result{algorithm, backend, scenario, workers, n, allowed.Load(), rejected.Load(), failures.Load(), percentile(.5), percentile(.95), percentile(.99), percentile(1), float64(n) / elapsed.Seconds()}
}

func limiter(algorithm, mode string, client *redis.Client, config rf.Config, capacity int) evaluator {
	var l evaluator
	var err error
	if mode == "ram" {
		c := fw.Config{Limit: config.Limit, Window: config.Window, MaxKeys: config.MaxKeys}
		switch algorithm {
		case "fixed_window":
			l, err = fw.New(c)
		case "sliding_window_log":
			l, err = sw.New(c)
		case "sliding_window_counter":
			l, err = sc.New(c)
		case "token_bucket":
			l, err = tb.New(tb.Config{Limit: c.Limit, Window: c.Window, Capacity: capacity, MaxKeys: c.MaxKeys})
		case "leaky_bucket":
			l, err = lb.New(lb.Config{Limit: c.Limit, Window: c.Window, Capacity: capacity, MaxKeys: c.MaxKeys})
		}
	} else {
		switch algorithm {
		case "fixed_window":
			l, err = rf.New(client, config)
		case "sliding_window_log":
			l, err = rs.New(client, config)
		case "sliding_window_counter":
			l, err = rc.New(client, config)
		case "token_bucket":
			l, err = rt.New(client, rt.Config{Config: config, Capacity: capacity})
		case "leaky_bucket":
			l, err = rl.New(client, rl.Config{Config: config, Capacity: capacity})
		}
	}
	must(err)
	if l == nil {
		panic("unknown algorithm")
	}
	if ready, ok := l.(interface{ Ready(context.Context) error }); ok {
		must(ready.Ready(context.Background()))
	}
	return l
}
func validate(r result, deny bool) {
	if r.Errors != 0 || (deny && r.Rejected != int64(r.Samples)) || (!deny && r.Allowed != int64(r.Samples)) {
		panic(fmt.Sprintf("scenario produced unexpected decisions: %+v", r))
	}
}
func main() {
	n := flag.Int("samples", 2000, "timed requests per scenario; excludes 100 warmups")
	output := flag.String("output", "reports/rate-limiter-latency.json", "JSON output file")
	flag.Parse()
	if *n < 100 || *n > 100000 {
		panic("samples must be between 100 and 100000")
	}
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		panic("REDIS_TEST_URL must point to isolated Redis")
	}
	opts, err := redis.ParseURL(url)
	must(err)
	opts.MaxRetries = -1
	opts.ContextTimeoutEnabled = true
	opts.PoolSize = 32
	opts.MaxActiveConns = 32
	opts.DialTimeout = time.Second
	opts.ReadTimeout = time.Second
	opts.WriteTimeout = time.Second
	client := redis.NewClient(opts)
	defer client.Close()
	ctx := context.Background()
	must(client.Ping(ctx).Err())
	namespace := fmt.Sprintf("algorithm-latency:%d", time.Now().UnixNano())
	defer func() {
		keys, err := client.Keys(ctx, namespace+":*").Result()
		if err == nil && len(keys) > 0 {
			must(client.Del(ctx, keys...).Err())
		}
	}()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	results := []result{}
	algorithms := []string{"fixed_window", "sliding_window_log", "sliding_window_counter", "token_bucket", "leaky_bucket"}
	caseID := 0
	for _, algorithm := range algorithms {
		fmt.Fprintln(os.Stderr, "Measuring", algorithm)
		for _, mode := range []string{"ram", "redis"} {
			for _, scenario := range []string{"allowed-hot", "denied-hot", "allowed-1000-identities", "capacity-full-1000-identities", "http-allowed", "http-denied"} {
				for _, workers := range []int{1, 16} {
					caseID++
					deny := scenario == "denied-hot" || scenario == "http-denied" || scenario == "capacity-full-1000-identities"
					limit := *n + 101
					if deny {
						limit = 1
					}
					config := rf.Config{PolicyID: fmt.Sprintf("case%d", caseID), Namespace: namespace, Limit: limit, Window: time.Hour, MaxKeys: 1000, Timeout: 5 * time.Second}
					var r result
					if strings.HasPrefix(scenario, "http-") {
						var upstreamCalls atomic.Int64
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { upstreamCalls.Add(1); w.WriteHeader(204) }))
						cfg := proxyapp.Config{Version: 1, Listen: ":8080", Upstream: upstream.URL, Storage: proxyapp.StorageConfig{Mode: mode, RedisURL: url, Namespace: namespace, Timeout: "5s"}, CleanupInterval: "1h", UpstreamTimeout: "5s", AnonymousTenant: "public", Routes: []proxyapp.Route{{Pattern: "/", Algorithm: algorithm, Policy: policy.Policy{ID: config.PolicyID, Identity: identity.IP}, Limit: limit, Window: "1h", Capacity: limit, MaxKeys: 1000}}}
						app, err := proxyapp.New(ctx, cfg, nil)
						must(err)
						server := httptest.NewServer(app.Handler())
						transport := &http.Transport{MaxIdleConns: 32, MaxIdleConnsPerHost: 32, MaxConnsPerHost: 32}
						httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}
						call := func(_ int) (bool, error) {
							response, err := httpClient.Get(server.URL)
							if err != nil {
								return false, err
							}
							_, err = io.Copy(io.Discard, response.Body)
							response.Body.Close()
							if err != nil {
								return false, err
							}
							if response.StatusCode != 204 && response.StatusCode != 429 {
								return false, fmt.Errorf("HTTP %d", response.StatusCode)
							}
							return response.StatusCode == 204, nil
						}
						if deny {
							ok, err := call(0)
							must(err)
							if !ok {
								panic("denial setup rejected")
							}
						}
						r = measure(algorithm, mode, scenario, workers, *n, call)
						wantUpstream := int64(*n + 100)
						if deny {
							wantUpstream = 1
						}
						if upstreamCalls.Load() != wantUpstream {
							panic("unexpected upstream traffic")
						}
						server.Close()
						app.Close()
						upstream.Close()
						transport.CloseIdleConnections()
					} else {
						l := limiter(algorithm, mode, client, config, limit)
						keys := make([]string, 1000)
						for i := range keys {
							keys[i] = fmt.Sprintf("u%d", i)
						}
						if scenario == "denied-hot" {
							d, err := l.Evaluate(ctx, "hot")
							must(err)
							if !d.Allowed {
								panic("denial setup rejected")
							}
						}
						if scenario == "capacity-full-1000-identities" {
							for _, key := range keys {
								d, err := l.Evaluate(ctx, key)
								must(err)
								if !d.Allowed {
									panic("capacity setup rejected")
								}
							}
						}
						call := func(i int) (bool, error) {
							key := "hot"
							if scenario == "allowed-1000-identities" {
								key = keys[i%1000]
							}
							if scenario == "capacity-full-1000-identities" {
								key = "overflow"
							}
							d, err := l.Evaluate(ctx, key)
							if err == nil && scenario == "capacity-full-1000-identities" && d.Reason != ratelimit.ReasonCapacityExceeded {
								return false, fmt.Errorf("expected capacity rejection, got %+v", d)
							}
							return d.Allowed, err
						}
						r = measure(algorithm, mode, scenario, workers, *n, call)
					}
					validate(r, deny)
					results = append(results, r)
				}
			}
		}
	}
	info, err := client.Info(ctx, "server").Result()
	must(err)
	version := "unknown"
	for _, line := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(line, "redis_version:") {
			version = strings.TrimPrefix(line, "redis_version:")
		}
	}
	report := struct {
		Schema       int       `json:"schema_version"`
		Timestamp    time.Time `json:"timestamp"`
		GOOS         string    `json:"goos"`
		GOARCH       string    `json:"goarch"`
		GoVersion    string    `json:"go_version"`
		CPUs         int       `json:"cpus"`
		GOMAXPROCS   int       `json:"gomaxprocs"`
		RedisVersion string    `json:"redis_version"`
		Methodology  string    `json:"methodology"`
		Warmups      int       `json:"warmup_requests"`
		Window       string    `json:"window"`
		AllowLimit   int       `json:"allowed_limit"`
		DenyLimit    int       `json:"denied_limit"`
		MaxKeys      int       `json:"max_keys"`
		Results      []result  `json:"results"`
	}{1, time.Now().UTC(), runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU(), runtime.GOMAXPROCS(0), version, "Single local run; closed-loop workers; no race instrumentation. Direct decisions include clock, lock and Redis TCP round trips. HTTP includes loopback proxy and a minimal 204 upstream for accepted requests. Percentiles use nearest rank. Hot log size grows during admission scenarios. No production network, TLS, failover or sustained-load claim.", 100, "1h", *n + 101, 1, 1000, results}
	data, err := json.MarshalIndent(report, "", "  ")
	must(err)
	must(os.MkdirAll(filepath.Dir(*output), 0755))
	// Only replace the report after all scenarios validate successfully.
	tmp := *output + ".tmp"
	must(os.WriteFile(tmp, append(data, '\n'), 0644))
	must(os.Rename(tmp, *output))
	fmt.Fprintln(os.Stderr, "Saved", len(results), "measurements to", *output)
}
