package proxyapp_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenBucketServerBinary(t *testing.T) {
	binary := os.Getenv("TOKEN_BUCKET_TEST_BINARY")
	if binary == "" {
		t.Skip("set TOKEN_BUCKET_TEST_BINARY to a built ratelimitd; make test-token-bucket builds it")
	}
	var calls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer backend.Close()
	cfg := configuration(backend.URL)
	tokenRedisConfig(t, &cfg)
	cfg.Routes[0].Algorithm = "token_bucket"
	cfg.Routes[0].Capacity = 7
	cfg.Routes[0].Window = "1h"
	cfg.Routes[0].Limit = 1
	var urls []string
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for replica := range 2 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		cfg.Listen = listener.Addr().String()
		listener.Close()
		directory := t.TempDir()
		configPath := filepath.Join(directory, "proxy.json")
		encoded, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(configPath, encoded, 0600); err != nil {
			t.Fatal(err)
		}
		log, err := os.Create(filepath.Join(directory, "server.log"))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, binary, "-config", configPath)
		cmd.Stdout, cmd.Stderr = log, log
		if err = cmd.Start(); err != nil {
			cancel()
			log.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		t.Cleanup(func() {
			cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				cancel()
				<-done
			}
			cancel()
			log.Close()
			if t.Failed() {
				data, _ := os.ReadFile(filepath.Join(directory, "server.log"))
				t.Logf("replica %d: %s", replica, data)
			}
		})
		url := "http://" + cfg.Listen
		deadline := time.Now().Add(5 * time.Second)
		for {
			response, err := client.Get(url + "/readyz")
			if err == nil {
				response.Body.Close()
				if response.StatusCode == 200 {
					break
				}
			}
			if time.Now().After(deadline) {
				t.Fatal("binary never became ready")
			}
			time.Sleep(20 * time.Millisecond)
		}
		urls = append(urls, url)
	}
	for i := range 100 {
		status, headers := tokenHTTPGet(t, client, urls[i%2]+"/", "")
		want := 204
		if i >= 7 {
			want = 429
		}
		if status != want {
			t.Fatalf("binary request %d status %d want %d", i, status, want)
		}
		if i >= 7 && headers.Get("Retry-After") == "" {
			t.Fatal("binary omitted retry hint")
		}
	}
	if calls.Load() != 7 {
		t.Fatalf("binary forwarded rejected requests: %d", calls.Load())
	}
}
