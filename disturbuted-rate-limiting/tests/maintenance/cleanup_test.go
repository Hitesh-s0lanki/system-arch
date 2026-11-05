package maintenance_test

import (
	"context"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/maintenance"
)

func TestCleanupLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}, 1), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- maintenance.RunCleanup(ctx, time.Millisecond, func() { started <- struct{}{}; <-release })
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("cleanup never ran")
	}
	cancel()
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not stop")
	}
}

func TestCleanupValidation(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second} {
		if err := maintenance.RunCleanup(context.Background(), interval, func() {}); err == nil {
			t.Fatal("invalid interval accepted")
		}
	}
	if err := maintenance.RunCleanup(context.Background(), time.Second, nil); err == nil {
		t.Fatal("nil cleanup accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := maintenance.RunCleanup(ctx, time.Hour, func() { t.Error("ran after cancellation") }); err != nil {
		t.Fatal(err)
	}
}
