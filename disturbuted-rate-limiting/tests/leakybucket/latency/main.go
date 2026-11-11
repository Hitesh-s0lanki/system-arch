// Command latency measures warmed leaky-bucket decisions and the local HTTP proxy.
// Run without -race. REDIS_TEST_URL must identify a disposable Redis instance.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	fixedredis "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow/redis"
	lb "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket"
	remote "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket/redis"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/proxyapp"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	redis "github.com/redis/go-redis/v9"
)

type result struct {
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
func measure(backend, scenario string, workers, n int, call func(int) (bool, error)) result {
	// Warm connections, scripts, map entries and HTTP transports before timing.
	for i := range 500 {
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
	return result{backend, scenario, workers, n, allowed.Load(), rejected.Load(), failures.Load(), percentile(.5), percentile(.95), percentile(.99), percentile(1), float64(n) / elapsed.Seconds()}
}

func main() {
	n := flag.Int("samples", 20000, "samples per normal scenario")
	output := flag.String("output", "", "JSON output file (defaults to stdout)")
	flag.Parse()
	if *n < 100 {
		panic("at least 100 samples required")
	}
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		panic("REDIS_TEST_URL required; use a disposable Redis")
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
	namespace := fmt.Sprintf("lb-latency:%d", time.Now().UnixNano())
	defer func() {
		keys, err := client.Keys(ctx, namespace+":*").Result()
		if err == nil && len(keys) > 0 {
			must(client.Del(ctx, keys...).Err())
		}
	}()
	results := []result{}
	caseID := 0
	for _, mode := range []string{"ram", "redis"} {
		for _, scenario := range []string{"allowed-hot", "denied-hot", "allowed-1000-identities", "capacity-full-10000-identities"} {
			for _, workers := range []int{1, 16} {
				caseID++
				count := *n
				cfg := lb.Config{Limit: 1, Window: time.Hour, Capacity: 1000000, MaxKeys: 10000}
				if scenario == "denied-hot" {
					cfg.Capacity = 1
				}
				if scenario == "capacity-full-10000-identities" {
					count = min(count, 2000)
					cfg.Capacity = 1
				}
				var l evaluator
				if mode == "ram" {
					local, e := lb.New(cfg)
					must(e)
					l = local
				} else {
					shared, e := remote.New(client, remote.Config{Config: fixedredis.Config{PolicyID: fmt.Sprintf("case%d", caseID), Namespace: namespace, Limit: cfg.Limit, Window: cfg.Window, MaxKeys: cfg.MaxKeys, Timeout: time.Second}, Capacity: cfg.Capacity})
					must(e)
					must(shared.Ready(ctx))
					l = shared
				}
				if scenario == "denied-hot" {
					_, err := l.Evaluate(ctx, "hot")
					must(err)
				}
				keys := make([]string, 10000)
				for i := range keys {
					keys[i] = fmt.Sprintf("u%d", i)
				}
				if scenario == "capacity-full-10000-identities" {
					for _, key := range keys {
						d, e := l.Evaluate(ctx, key)
						must(e)
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
					if scenario == "capacity-full-10000-identities" {
						key = "overflow"
					}
					d, e := l.Evaluate(ctx, key)
					return d.Allowed, e
				}
				r := measure(mode, scenario, workers, count, call)
				if r.Errors != 0 || ((scenario == "denied-hot" || scenario == "capacity-full-10000-identities") && r.Rejected != int64(count)) || ((scenario == "allowed-hot" || scenario == "allowed-1000-identities") && r.Allowed != int64(count)) {
					panic(fmt.Sprintf("unexpected results %+v", r))
				}
				results = append(results, r)
			}
		}
	}
	// Measure a full TCP HTTP request through the proxy. Allowed requests include
	// the round trip to a minimal local upstream; rejections do not reach upstream.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, mode := range []string{"ram", "redis"} {
		for _, deny := range []bool{false, true} {
			for _, workers := range []int{1, 16} {
				caseID++
				capacity := 1000000
				if deny {
					capacity = 1
				}
				var upstreamCalls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { upstreamCalls.Add(1); w.WriteHeader(204) }))
				cfg := proxyapp.Config{Version: 1, Listen: ":8080", Upstream: upstream.URL, Storage: proxyapp.StorageConfig{Mode: mode, RedisURL: url, Namespace: namespace, Timeout: "1s"}, CleanupInterval: "1s", UpstreamTimeout: "5s", AnonymousTenant: "public", Routes: []proxyapp.Route{{Pattern: "/", Algorithm: "leaky_bucket", Policy: policy.Policy{ID: fmt.Sprintf("case%d", caseID), Identity: identity.IP}, Limit: 1, Window: "1h", Capacity: capacity, MaxKeys: 10}}}
				app, e := proxyapp.New(ctx, cfg, nil)
				must(e)
				server := httptest.NewServer(app.Handler())
				transport := &http.Transport{MaxIdleConns: 32, MaxIdleConnsPerHost: 32, MaxConnsPerHost: 32}
				httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}
				call := func(_ int) (bool, error) {
					response, e := httpClient.Get(server.URL)
					if e != nil {
						return false, e
					}
					_, e = io.Copy(io.Discard, response.Body)
					response.Body.Close()
					if e != nil {
						return false, e
					}
					if response.StatusCode != 204 && response.StatusCode != 429 {
						return false, fmt.Errorf("HTTP %d", response.StatusCode)
					}
					return response.StatusCode == 204, nil
				}
				if deny {
					_, e := call(0)
					must(e)
				}
				scenario := "http-allowed"
				if deny {
					scenario = "http-denied"
				}
				count := min(*n, 5000)
				r := measure(mode, scenario, workers, count, call)
				if r.Errors != 0 || (deny && r.Rejected != int64(count)) || (!deny && r.Allowed != int64(count)) {
					panic(fmt.Sprintf("unexpected HTTP results %+v", r))
				}
				if deny && upstreamCalls.Load() != 1 {
					panic("rejected requests reached upstream")
				}
				results = append(results, r)
				server.Close()
				app.Close()
				upstream.Close()
				transport.CloseIdleConnections()
			}
		}
	}
	info, err := client.Info(ctx, "server").Result()
	must(err)
	report := struct {
		Timestamp   time.Time `json:"timestamp"`
		GOOS        string    `json:"goos"`
		GOARCH      string    `json:"goarch"`
		GoVersion   string    `json:"go_version"`
		CPUs        int       `json:"cpus"`
		RedisServer string    `json:"redis_server_info"`
		Results     []result  `json:"results"`
	}{time.Now().UTC(), runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU(), info, results}
	data, err := json.MarshalIndent(report, "", "  ")
	must(err)
	if *output != "" {
		must(os.WriteFile(*output, append(data, '\n'), 0644))
	} else {
		fmt.Println(string(data))
	}
}
