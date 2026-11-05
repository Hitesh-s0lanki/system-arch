package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/jobs"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/model"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/results"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/storage"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

type API struct {
	Jobs    *jobs.Service
	DataDir string
}

var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (a *API) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	m.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if a.Jobs.Runner.Ready(ctx) != nil {
			fail(w, 503, "Docker Engine unavailable")
			return
		}
		write(w, 200, map[string]string{"status": "ready"})
	})
	m.HandleFunc("POST /tests", a.create)
	m.HandleFunc("GET /tests", a.list)
	m.HandleFunc("GET /tests/{id}", a.get)
	m.HandleFunc("POST /tests/{id}/cancel", a.cancel)
	m.HandleFunc("GET /tests/{id}/results", a.report)
	m.HandleFunc("GET /tests/{id}/metrics", a.metrics)
	m.HandleFunc("GET /tests/{id}/logs", a.logs)
	return m
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	write(w, status, map[string]string{"error": message})
}
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var c model.Config
	if e := d.Decode(&c); e != nil {
		fail(w, 400, "invalid JSON: "+e.Error())
		return
	}
	if d.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "body must contain one JSON object")
		return
	}
	if e := c.Validate(); e != nil {
		fail(w, 400, e.Error())
		return
	}
	j, e := a.Jobs.Create(c)
	if e != nil {
		fail(w, 503, "unable to queue test")
		return
	}
	w.Header().Set("Location", "/tests/"+j.ID)
	write(w, 202, j.Public())
}
func (a *API) job(w http.ResponseWriter, r *http.Request) (model.Job, bool) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		fail(w, 404, "test not found")
		return model.Job{}, false
	}
	j, e := a.Jobs.Store.Get(id)
	if errors.Is(e, storage.ErrNotFound) {
		fail(w, 404, e.Error())
		return j, false
	}
	if e != nil {
		fail(w, 500, "storage error")
		return j, false
	}
	return j, true
}
func (a *API) get(w http.ResponseWriter, r *http.Request) {
	if j, ok := a.job(w, r); ok {
		write(w, 200, j.Public())
	}
}
func page(r *http.Request) (int, int, error) {
	limit, offset := 100, 0
	var e error
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, e = strconv.Atoi(v)
		if e != nil {
			return 0, 0, e
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, e = strconv.Atoi(v)
		if e != nil {
			return 0, 0, e
		}
	}
	if limit < 1 || limit > 1000 || offset < 0 {
		return 0, 0, errors.New("limit must be 1–1000 and offset nonnegative")
	}
	return limit, offset, nil
}
func (a *API) list(w http.ResponseWriter, r *http.Request) {
	limit, offset, e := page(r)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	items, e := a.Jobs.Store.List(limit, offset)
	if e != nil {
		fail(w, 500, "storage error")
		return
	}
	for i := range items {
		items[i] = items[i].Public()
	}
	write(w, 200, map[string]any{"items": items, "offset": offset, "limit": limit})
}
func (a *API) cancel(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.job(w, r); !ok {
		return
	}
	j, e := a.Jobs.Cancel(r.PathValue("id"))
	if errors.Is(e, jobs.ErrConflict) {
		fail(w, 409, e.Error())
		return
	}
	if e != nil {
		fail(w, 500, "unable to cancel test")
		return
	}
	write(w, 202, j.Public())
}
func (a *API) finished(w http.ResponseWriter, r *http.Request) (model.Job, bool) {
	j, ok := a.job(w, r)
	if !ok {
		return j, false
	}
	if j.Status == "queued" || j.Status == "running" || j.Status == "cancelling" {
		fail(w, 409, "test has not finished")
		return j, false
	}
	return j, true
}
func (a *API) report(w http.ResponseWriter, r *http.Request) {
	j, ok := a.finished(w, r)
	if !ok {
		return
	}
	report, e := results.ReadForJob(filepath.Join(a.DataDir, j.ID), j)
	if e != nil {
		fail(w, 409, "complete results unavailable; inspect job status and logs")
		return
	}
	write(w, 200, map[string]any{"test": j.Public(), "results": report})
}
func (a *API) metrics(w http.ResponseWriter, r *http.Request) {
	j, ok := a.finished(w, r)
	if !ok {
		return
	}
	limit, offset, e := page(r)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	points, more, e := results.Points(filepath.Join(a.DataDir, j.ID, "metrics.json"), offset, limit)
	if e != nil {
		fail(w, 409, "metrics unavailable or incomplete")
		return
	}
	write(w, 200, map[string]any{"items": points, "offset": offset, "limit": limit, "hasMore": more, "nextOffset": offset + len(points)})
}
func (a *API) logs(w http.ResponseWriter, r *http.Request) {
	j, ok := a.finished(w, r)
	if !ok {
		return
	}
	b, e := os.ReadFile(filepath.Join(a.DataDir, j.ID, "logs.txt"))
	if e != nil {
		fail(w, 409, "logs unavailable")
		return
	}
	write(w, 200, map[string]string{"logs": string(b)})
}
