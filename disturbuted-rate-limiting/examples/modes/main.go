// A local Docker playground demonstrating service-level and gateway-level SDK use.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/sdk"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
)

func handler(role, mode string, upstreams map[string]string) (http.Handler, error) {
	if mode != "sdk" && mode != "gateway" {
		return nil, fmt.Errorf("invalid demo mode")
	}
	if role != "gateway" && role != "auth" && role != "ads" && role != "keywords" {
		return nil, fmt.Errorf("invalid demo role")
	}
	embedded := (mode == "sdk" && role == "keywords") || (mode == "gateway" && role == "gateway")
	var limiter *sdk.Client
	if embedded {
		var err error
		limiter, err = sdk.NewMemory([]sdk.Policy{{
			Policy:      policy.Policy{ID: "demo", Identity: identity.User},
			FixedWindow: fixedwindow.Config{Limit: 3, Window: time.Minute, MaxKeys: 100},
		}})
		if err != nil {
			return nil, err
		}
	}
	proxies := map[string]*httputil.ReverseProxy{}
	if role == "gateway" {
		for name, address := range upstreams {
			target, err := url.Parse(address)
			if err != nil {
				return nil, err
			}
			proxy := httputil.NewSingleHostReverseProxy(target)
			proxy.Transport = &http.Transport{ResponseHeaderTimeout: 5 * time.Second}
			proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
				http.Error(w, "Demo service unavailable", http.StatusBadGateway)
			}
			proxies[name] = proxy
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		destination := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
		if role == "gateway" && proxies[destination] == nil {
			http.NotFound(w, r)
			return
		}
		if limiter != nil {
			// One server-selected demo user makes quota scope visible across service routes.
			decision, err := limiter.Check(r.Context(), "demo", service.Request{TenantID: "demo", UserID: "demo-user"})
			w.Header().Set("X-Demo-Enforcement", role)
			w.Header().Set("X-RateLimit-Limit", "3")
			if err != nil {
				http.Error(w, "Limiter unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprint(decision.Remaining))
			if !decision.Allowed {
				w.Header().Set("Retry-After", fmt.Sprint(int(math.Ceil(decision.RetryAfter.Seconds()))))
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "Rate limited", "enforced_by": role, "service": destination})
				return
			}
		}
		if role == "gateway" {
			proxies[destination].ServeHTTP(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"service": role, "mode": mode, "sdk_embedded": embedded, "path": r.URL.Path})
	}), nil
}

func main() {
	check := flag.Bool("healthcheck", false, "Check the local readiness endpoint")
	flag.Parse()
	if *check {
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:8080/readyz")
		if err != nil {
			os.Exit(1)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	app, err := handler(os.Getenv("DEMO_ROLE"), os.Getenv("DEMO_MODE"), map[string]string{
		"auth": "http://auth:8080", "ads": "http://ads:8080", "keywords": "http://keywords:8080",
	})
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: ":8080", Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: time.Minute}
	log.Fatal(server.ListenAndServe())
}
