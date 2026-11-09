// Package testredis owns isolated Redis processes for failure tests. It never
// mutates a developer's Redis. Set REDIS_SERVER_BIN to enable these tests.
package testredis

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func Address(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}
func Start(t *testing.T, address, extra string, sentinel bool) *redis.Client {
	t.Helper()
	binary := os.Getenv("REDIS_SERVER_BIN")
	if binary == "" {
		t.Skip("set REDIS_SERVER_BIN to run isolated Redis failure tests")
	}
	_, port, _ := net.SplitHostPort(address)
	directory := t.TempDir()
	config := fmt.Sprintf("bind 127.0.0.1\nport %s\nprotected-mode yes\ndir %s\nsave \"\"\nappendonly no\n%s\n", port, directory, extra)
	path := filepath.Join(directory, "redis.conf")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{path}
	if sentinel {
		args = append(args, "--sentinel")
	}
	logPath := filepath.Join(directory, "redis.log")
	file, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = file
	cmd.Stderr = file
	if err := cmd.Start(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = file.Close()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Log(string(data))
		}
	})
	client := redis.NewClient(&redis.Options{Addr: address, MaxRetries: -1, ReadTimeout: time.Second, WriteTimeout: time.Second, ContextTimeoutEnabled: true})
	t.Cleanup(func() { client.Close() })
	Eventually(t, 5*time.Second, func() bool { return client.Ping(context.Background()).Err() == nil })
	return client
}
func Eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("Redis condition did not become true before deadline")
}
