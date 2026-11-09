package proxyapp_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/proxyapp"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/tests/testredis"
	redis "github.com/redis/go-redis/v9"
)

func TestRedisReadinessOOMAndRevisionMigration(t *testing.T) {
	client := testredis.Start(t, testredis.Address(t), "", false)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer backend.Close()
	cfg := configuration(backend.URL)
	cfg.Storage.Mode = "redis"
	cfg.Storage.RedisURL = "redis://" + client.Options().Addr
	cfg.Routes[0].Limit = 1
	a, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	request := func(app *proxyapp.App, path string) int {
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code
	}
	// Probes must not spend allowance. Conflicts fail startup before any request.
	if request(a, "/readyz") != 200 || request(a, "/readyz") != 200 {
		t.Fatal("not ready")
	}
	changed := cfg
	changed.Routes = append([]proxyapp.Route(nil), cfg.Routes...)
	changed.Routes[0].Limit = 2
	if b, err := proxyapp.New(context.Background(), changed, nil); err == nil {
		b.Close()
		t.Fatal("same revision accepted conflicting settings")
	}
	if request(a, "/") != 204 || request(a, "/") != 429 {
		t.Fatal("probe consumed quota or quota not enforced")
	}
	changed.Routes[0].Revision = "v2"
	b, err := proxyapp.New(context.Background(), changed, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if request(b, "/") != 204 || request(a, "/") != 429 {
		t.Fatal("revision reset leaked across policies")
	}
	// PING succeeds during OOM; readiness must exercise actual writes.
	if err := client.ConfigSet(context.Background(), "maxmemory", "1").Err(); err != nil {
		t.Fatal(err)
	}
	if client.Ping(context.Background()).Err() != nil {
		t.Fatal("OOM unexpectedly broke ping")
	}
	if request(a, "/readyz") != 503 || request(a, "/") != 503 {
		t.Fatal("OOM did not fail closed")
	}
	if err := client.ConfigSet(context.Background(), "maxmemory", "0").Err(); err != nil {
		t.Fatal(err)
	}
	if request(a, "/readyz") != 200 {
		t.Fatal("readiness did not recover")
	}
	// Convert isolated Redis into a read-only replica with an unavailable primary.
	if err := client.ReplicaOf(context.Background(), "127.0.0.1", "1").Err(); err != nil {
		t.Fatal(err)
	}
	if request(a, "/readyz") != 503 || request(a, "/") != 503 {
		t.Fatal("read-only primary did not fail closed")
	}
}

func TestSentinelPromotionPreservesReplicatedQuota(t *testing.T) {
	primaryAddress := testredis.Address(t)
	primary := testredis.Start(t, primaryAddress, "", false)
	host, port, _ := net.SplitHostPort(primaryAddress)
	replicaAddress := testredis.Address(t)
	replica := testredis.Start(t, replicaAddress, fmt.Sprintf("replicaof %s %s\nrepl-diskless-sync-delay 0", host, port), false)
	sentinelAddress := testredis.Address(t)
	testredis.Start(t, sentinelAddress, fmt.Sprintf("sentinel monitor test %s %s 1\nsentinel down-after-milliseconds test 500\nsentinel failover-timeout test 5000", host, port), true)
	sentinel := redis.NewSentinelClient(&redis.Options{Addr: sentinelAddress})
	defer sentinel.Close()
	testredis.Eventually(t, 10*time.Second, func() bool {
		info, _ := replica.Info(context.Background(), "replication").Result()
		replicas, _ := sentinel.Replicas(context.Background(), "test").Result()
		return strings.Contains(info, "master_link_status:up") && len(replicas) == 1
	})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer backend.Close()
	cfg := configuration(backend.URL)
	cfg.Storage.Mode = "redis"
	cfg.Storage.RedisURL = "redis://" + primaryAddress
	cfg.Storage.Topology = "sentinel"
	cfg.Storage.Addresses = []string{sentinelAddress}
	cfg.Storage.MasterName = "test"
	cfg.Storage.Timeout = "1s"
	cfg.Routes[0].Limit = 1
	a, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	// Wait for the replica's actual offset, rather than assuming replication is durable.
	testredis.Eventually(t, 5*time.Second, func() bool {
		p, _ := primary.Info(context.Background(), "replication").Result()
		r, _ := replica.Info(context.Background(), "replication").Result()
		offset := func(s string) int64 {
			for _, line := range strings.Split(s, "\r\n") {
				if strings.HasPrefix(line, "master_repl_offset:") {
					v, _ := strconv.ParseInt(strings.TrimPrefix(line, "master_repl_offset:"), 10, 64)
					return v
				}
			}
			return -1
		}
		return offset(p) >= 0 && offset(r) >= offset(p)
	})
	// Stop only this fixture's primary and let Sentinel detect the outage.
	if err := primary.ShutdownNoSave(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	testredis.Eventually(t, 15*time.Second, func() bool {
		address, err := sentinel.GetMasterAddrByName(context.Background(), "test").Result()
		return err == nil && net.JoinHostPort(address[0], address[1]) == replicaAddress
	})
	testredis.Eventually(t, 10*time.Second, func() bool {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		return w.Code == 429
	})
}

func TestClusterSharedQuota(t *testing.T) {
	addresses := []string{testredis.Address(t), testredis.Address(t), testredis.Address(t)}
	clients := []*redis.Client{}
	busPorts := []string{}
	for _, address := range addresses {
		_, busPort, _ := net.SplitHostPort(testredis.Address(t))
		busPorts = append(busPorts, busPort)
		clients = append(clients, testredis.Start(t, address, "cluster-enabled yes\ncluster-config-file nodes.conf\ncluster-node-timeout 1000\ncluster-port "+busPort, false))
	}
	for i, address := range addresses[1:] {
		host, port, _ := net.SplitHostPort(address)
		if err := clients[0].Do(context.Background(), "CLUSTER", "MEET", host, port, busPorts[i+1]).Err(); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range clients {
		start := i * 16384 / 3
		end := (i+1)*16384/3 - 1
		if err := c.ClusterAddSlotsRange(context.Background(), start, end).Err(); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range clients {
		testredis.Eventually(t, 10*time.Second, func() bool {
			info, _ := c.ClusterInfo(context.Background()).Result()
			return strings.Contains(info, "cluster_state:ok")
		})
	}
	testredis.CheckAmbiguousClusterReply(t, addresses)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer backend.Close()
	cfg := configuration(backend.URL)
	cfg.Storage.Mode = "redis"
	cfg.Storage.RedisURL = "redis://" + addresses[0]
	cfg.Storage.Topology = "cluster"
	cfg.Storage.Addresses = addresses
	cfg.Storage.Timeout = "1s"
	cfg.Routes[0].Limit = 1
	first, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := proxyapp.New(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for i, a := range []*proxyapp.App{first, second} {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		want := 204
		if i == 1 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("Cluster request %d got %d want %d", i, w.Code, want)
		}
	}
}
