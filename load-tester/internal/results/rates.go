package results

import (
	"fmt"
	"math"
	"strconv"

	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/model"
)

// These minimums are evidence gates for a provisional suggestion, not statistical
// confidence guarantees. A longer, representative run is still needed to deploy it.
const minimumStepSeconds = 30
const minimumSuccessfulRequests = 100

type RateObservation struct {
	RequestedRPS       int              `json:"requestedRps"`
	ObservedRPS        float64          `json:"observedRps"`
	DurationSeconds    int              `json:"durationSeconds"`
	Requests           int64            `json:"requests"`
	SuccessfulRequests int64            `json:"successfulRequests"`
	StatusCounts       map[string]int64 `json:"statusCounts"`
	ErrorRate          float64          `json:"errorRate"`
	SuccessfulP95MS    *float64         `json:"successfulP95Ms"`
	DroppedIterations  int64            `json:"droppedIterations"`
	Status             string           `json:"status"`
	Reasons            []string         `json:"reasons"`
}

type RateAnalysis struct {
	Steps                   []RateObservation `json:"steps"`
	HighestPassingRPS       int               `json:"highestPassingRps"`
	RecommendedRateLimitRPS *int              `json:"recommendedRateLimitRps"`
	HeadroomPercent         int               `json:"headroomPercent"`
	Boundary                string            `json:"boundary"`
	Scope                   string            `json:"scope"`
	Explanation             string            `json:"explanation"`
}

func analyzeRates(report Report, c model.Config, completed bool) *RateAnalysis {
	headroom := c.HeadroomPercent
	if headroom == 0 {
		headroom = 20
	}
	a := &RateAnalysis{Steps: []RateObservation{}, HeadroomPercent: headroom, Boundary: "lower_bound", Scope: "aggregate traffic for the tested endpoint, workload, and client identity; not a per-user quota"}
	contiguous := true
	for i, step := range c.Steps() {
		scenario := c.Scenario(i)
		counts := report.scenarioCounts[scenario]
		if counts == nil {
			counts = map[string]int64{}
		}
		o := RateObservation{RequestedRPS: step.RequestsPerSecond, DurationSeconds: step.DurationSeconds, StatusCounts: counts, Status: "passed", Reasons: []string{}}
		for status, count := range counts {
			o.Requests += count
			code, _ := strconv.Atoi(status)
			if code >= 200 && code < 300 {
				o.SuccessfulRequests += count
			}
		}
		o.ObservedRPS = float64(o.Requests) / float64(step.DurationSeconds)
		if o.Requests > 0 {
			o.ErrorRate = float64(o.Requests-o.SuccessfulRequests) / float64(o.Requests)
		}
		suffix := "{scenario:" + scenario + "}"
		latency := report.Metrics["successful_latency"+suffix].Values
		if p95, ok := latency["p(95)"]; ok && o.SuccessfulRequests > 0 {
			o.SuccessfulP95MS = &p95
		}
		o.DroppedIterations = int64(report.Metrics["dropped_iterations"+suffix].Values["count"])
		if o.ErrorRate > c.MaxErrorRate {
			o.Status = "failed"
			o.Reasons = append(o.Reasons, "Unsuccessful responses exceeded maxErrorRate; 429s count as unsuccessful.")
		}
		if o.SuccessfulP95MS != nil && c.P95ThresholdMS > 0 && *o.SuccessfulP95MS > c.P95ThresholdMS {
			o.Status = "failed"
			o.Reasons = append(o.Reasons, "Successful-request p95 exceeded p95ThresholdMs.")
		}
		if !completed || o.DroppedIterations > 0 || o.Requests < int64(math.Ceil(float64(step.RequestsPerSecond*step.DurationSeconds)*0.95)) {
			o.Status = "inconclusive"
			o.Reasons = append(o.Reasons, "Incomplete measurement or the generator did not deliver the requested load; this does not establish target capacity.")
		} else if o.Status != "failed" && (step.DurationSeconds < minimumStepSeconds || o.SuccessfulRequests < minimumSuccessfulRequests || o.SuccessfulP95MS == nil || c.P95ThresholdMS <= 0) {
			o.Status = "inconclusive"
			o.Reasons = append(o.Reasons, "A suggestion requires a latency threshold, at least 30 seconds and 100 successful requests per passing step.")
		}
		if contiguous && o.Status == "passed" {
			a.HighestPassingRPS = step.RequestsPerSecond
		} else {
			contiguous = false
		}
		if a.Boundary == "lower_bound" && o.Status != "passed" {
			if o.Status == "failed" {
				a.Boundary = "threshold_crossed"
			} else {
				a.Boundary = "inconclusive"
			}
		}
		a.Steps = append(a.Steps, o)
	}
	if a.HighestPassingRPS > 0 {
		suggestion := int(math.Floor(float64(a.HighestPassingRPS) * float64(100-headroom) / 100))
		if suggestion >= 1 {
			a.RecommendedRateLimitRPS = &suggestion
		}
	}
	if a.RecommendedRateLimitRPS == nil {
		a.Explanation = "No rate limit suggested: sufficient passing evidence was not obtained at the initial contiguous load steps. Review the step reasons and rerun with adequate duration, samples, and generator capacity."
	} else {
		a.Explanation = fmt.Sprintf("Provisional aggregate limit: %d requests/second, using %d%% headroom below the highest consecutively passing tested rate (%d RPS). Validate with longer representative traffic before applying it; no server configuration was changed.", *a.RecommendedRateLimitRPS, headroom, a.HighestPassingRPS)
		if a.Boundary == "lower_bound" {
			a.Explanation += " All tested rates passed; the capacity ceiling was not found."
		}
	}
	return a
}
