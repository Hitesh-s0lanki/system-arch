// Package httpserver owns the HTTP and maintenance lifecycle.
package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/maintenance"
)

func Serve(ctx context.Context, listener net.Listener, handler http.Handler, interval, writeTimeout time.Duration, cleanup func()) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: writeTimeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	maintenanceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cleanupDone := make(chan error, 1)
	go func() { cleanupDone <- maintenance.RunCleanup(maintenanceCtx, interval, cleanup) }()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	var serveErr error
	served := false
	select {
	case <-ctx.Done():
	case serveErr = <-serveDone:
		served = true
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = server.Close()
	}
	if !served {
		serveErr = <-serveDone
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr, <-cleanupDone)
}
