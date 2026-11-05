// Package maintenance provides caller-owned background maintenance.
package maintenance

import (
	"context"
	"fmt"
	"time"
)

// RunCleanup calls cleanup periodically until cancellation. Callers own the
// goroutine and must wait for this function to return during shutdown.
func RunCleanup(ctx context.Context, interval time.Duration, cleanup func()) error {
	if interval <= 0 || cleanup == nil {
		return fmt.Errorf("maintenance: positive interval and cleanup function required")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if ctx.Err() == nil {
				cleanup()
			}
		}
	}
}
