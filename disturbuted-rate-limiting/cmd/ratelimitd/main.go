package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/app"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/internal/proxyapp"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	var config app.Config
	configFile := flag.String("config", "", "version 1 reverse-proxy JSON configuration")
	healthURL := flag.String("healthcheck", "", "check a health URL and exit")
	address := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	flag.IntVar(&config.Limit, "limit", 3, "allowed requests per identity and window")
	flag.DurationVar(&config.Window, "window", 10*time.Second, "fixed window duration")
	flag.IntVar(&config.MaxKeys, "max-keys", 10000, "key capacity per policy")
	flag.DurationVar(&config.CleanupInterval, "cleanup-interval", time.Second, "periodic cleanup interval")
	flag.Func("trusted-proxy", "trusted proxy CIDR (repeatable; empty trusts none)", func(value string) error {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return err
		}
		config.TrustedProxies = append(config.TrustedProxies, prefix)
		return nil
	})
	flag.Parse()
	if *healthURL != "" {
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Get(*healthURL)
		if err != nil {
			return fmt.Errorf("health check failed")
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("health check failed: status %d", response.StatusCode)
		}
		return nil
	}
	if raw := os.Getenv("RATE_LIMIT_CREDENTIALS"); raw != "" {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config.Credentials); err != nil {
			return fmt.Errorf("invalid RATE_LIMIT_CREDENTIALS JSON")
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return fmt.Errorf("RATE_LIMIT_CREDENTIALS must contain one JSON array")
		}
	}
	// No credentials means authenticated routes reject requests; no built-in token.
	if config.Credentials == nil {
		config.Credentials = []auth.Credential{}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *configFile != "" {
		proxyConfig, err := proxyapp.Load(*configFile)
		if err != nil {
			return err
		}
		flag.Visit(func(value *flag.Flag) {
			if value.Name == "addr" {
				proxyConfig.Listen = *address
			}
		})
		a, err := proxyapp.New(ctx, proxyConfig, config.Credentials)
		if err != nil {
			return err
		}
		defer a.Close()
		listener, err := net.Listen("tcp", proxyConfig.Listen)
		if err != nil {
			return err
		}
		log.Printf("rate-limit proxy listening on %s (storage=%s)", listener.Addr(), proxyConfig.Storage.Mode)
		return a.Serve(ctx, listener)
	}
	a, err := app.New(config)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return err
	}
	log.Printf("rate limiter listening on %s", listener.Addr())
	return a.Serve(ctx, listener)
}
