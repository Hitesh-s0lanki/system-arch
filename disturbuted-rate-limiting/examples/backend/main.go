// A private demonstration upstream for Docker Compose. Not a production backend.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

func main() {
	var requests atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "body too large", 413)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"backend": "demo", "requests": requests.Add(1), "method": r.Method, "path": r.URL.Path,
			"query": r.URL.RawQuery, "body": string(body), "client_ip": r.Header.Get("X-Forwarded-For"),
			"tenant_id": r.Header.Get("X-RateLimit-Tenant-ID"), "user_id": r.Header.Get("X-RateLimit-User-ID"),
		})
	})
	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: time.Minute}
	log.Fatal(server.ListenAndServe())
}
