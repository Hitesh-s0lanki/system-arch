// Package telemetry exposes bounded-cardinality operational metrics.
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	ratelimit "github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry       *prometheus.Registry
	Decisions      *prometheus.CounterVec
	Latency        *prometheus.HistogramVec
	HTTP           *prometheus.CounterVec
	UpstreamErrors *prometheus.CounterVec
	Inflight       prometheus.Gauge
	Capacity       *prometheus.GaugeVec
}

func New() *Metrics {
	m := &Metrics{
		Registry:       prometheus.NewRegistry(),
		Decisions:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ratelimit_decisions_total", Help: "Limiter decisions by configured policy and outcome."}, []string{"policy", "outcome"}),
		Latency:        prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "ratelimit_decision_duration_seconds", Help: "Time spent evaluating a decision.", Buckets: []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1}}, []string{"policy"}),
		HTTP:           prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ratelimit_http_requests_total", Help: "Requests by configured route and HTTP status."}, []string{"route", "status"}),
		UpstreamErrors: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ratelimit_upstream_errors_total", Help: "Upstream failures by bounded reason."}, []string{"reason"}),
		Inflight:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "ratelimit_inflight_requests", Help: "Admitted concurrent gateway requests."}),
		Capacity:       prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "ratelimit_capacity_utilization_ratio", Help: "Maximum tenant capacity utilization by policy."}, []string{"policy"}),
	}
	m.Registry.MustRegister(m.Decisions, m.Latency, m.HTTP, m.UpstreamErrors, m.Inflight, m.Capacity, prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	return m
}
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

type Checker struct {
	Service *service.Service
	Metrics *Metrics
}

func (c Checker) Check(ctx context.Context, policy string, request service.Request) (ratelimit.Decision, error) {
	start := time.Now()
	d, err := c.Service.Check(ctx, policy, request)
	outcome := string(d.Reason)
	if d.Allowed {
		outcome = "allowed"
	}
	if err != nil {
		outcome = "backend_error"
		slog.Warn("rate limiter decision failed", "policy", policy, "error", err)
		if errors.Is(err, service.ErrUnauthorizedTenant) {
			outcome = "unauthorized_tenant"
		}
	}
	c.Metrics.Decisions.WithLabelValues(policy, outcome).Inc()
	c.Metrics.Latency.WithLabelValues(policy).Observe(time.Since(start).Seconds())
	return d, err
}
