package main

import (
	"context"
	"errors"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/api"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/config"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/docker"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/jobs"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/storage"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
func run() error {
	c, e := config.Load()
	if e != nil {
		return e
	}
	store, e := storage.Open(filepath.Join(c.DataDir, "jobs.sqlite"))
	if e != nil {
		return e
	}
	defer store.Close()
	runner := &docker.Runner{Config: c}
	service := jobs.New(store, runner)
	recoverCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	e = service.Recover(recoverCtx)
	cancel()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: c.Address, Handler: (&api.API{Jobs: service, DataDir: c.DataDir}).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	workerDone := make(chan error, 1)
	go func() {
		e := service.Work(ctx)
		workerDone <- e
		if e != nil {
			stop()
		}
	}()
	serverDone := make(chan error, 1)
	go func() {
		log.Printf("load tester listening on %s", c.Address)
		serverDone <- server.ListenAndServe()
		stop()
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	workerErr := <-workerDone
	serverErr := <-serverDone
	if workerErr != nil {
		return workerErr
	}
	if !errors.Is(serverErr, http.ErrServerClosed) {
		return serverErr
	}
	return nil
}
