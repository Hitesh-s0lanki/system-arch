package results

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/model"
	"os"
	"path/filepath"
)

type Metric struct {
	Values     map[string]float64 `json:"values"`
	Thresholds map[string]struct {
		OK bool `json:"ok"`
	} `json:"thresholds"`
}
type Report struct {
	RateAnalysis   *RateAnalysis `json:"rateAnalysis,omitempty"`
	scenarioCounts map[string]map[string]int64
	Metrics        map[string]Metric `json:"metrics"`
	StatusCounts   map[string]int64  `json:"statusCounts"`
	Insights       []string          `json:"insights"`
}

func Read(dir string) (Report, error) {
	var r Report
	b, e := os.ReadFile(filepath.Join(dir, "summary.json"))
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return r, e
	}
	r.StatusCounts = map[string]int64{}
	r.scenarioCounts = map[string]map[string]int64{}
	r.Insights = []string{}
	f, e := os.Open(filepath.Join(dir, "metrics.json"))
	if e != nil {
		return r, e
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 65536), 1024*1024)
	for scanner.Scan() {
		var p struct {
			Type   string `json:"type"`
			Metric string `json:"metric"`
			Data   struct {
				Value float64           `json:"value"`
				Tags  map[string]string `json:"tags"`
			} `json:"data"`
		}
		if e = json.Unmarshal(scanner.Bytes(), &p); e != nil {
			return r, e
		}
		if p.Type == "Point" && p.Metric == "response_status" {
			r.StatusCounts[p.Data.Tags["status"]] += int64(p.Data.Value)
			scenario := p.Data.Tags["scenario"]
			if r.scenarioCounts[scenario] == nil {
				r.scenarioCounts[scenario] = map[string]int64{}
			}
			r.scenarioCounts[scenario][p.Data.Tags["status"]] += int64(p.Data.Value)
		}
	}
	if e = scanner.Err(); e != nil {
		return r, e
	}
	for name, m := range r.Metrics {
		for threshold, t := range m.Thresholds {
			if !t.OK {
				r.Insights = append(r.Insights, fmt.Sprintf("Threshold failed: %s %s", name, threshold))
			}
		}
	}
	if r.StatusCounts["429"] > 0 {
		r.Insights = append(r.Insights, "Rate limiting occurred; compare successful_latency separately from rate_limited_latency.")
	}
	if r.StatusCounts["0"] > 0 {
		r.Insights = append(r.Insights, "Some requests received no HTTP response; inspect transport_failures and request_timeouts.")
	}
	if r.Metrics["successful_latency"].Values["count"] == 0 {
		r.Insights = append(r.Insights, "No successful requests were measured; successful-request percentiles are unavailable.")
	}
	r.Insights = append(r.Insights, "This run measures the selected workload; it does not establish maximum server capacity or identify internal bottlenecks.")
	return r, nil
}

// ReadForJob adds rate-limit guidance only to a completed arrival-rate measurement.
func ReadForJob(dir string, j model.Job) (Report, error) {
	r, e := Read(dir)
	if e != nil {
		return r, e
	}
	if j.Config.ArrivalRate() {
		r.RateAnalysis = analyzeRates(r, j.Config, j.Status == "completed")
		r.Insights = append(r.Insights, r.RateAnalysis.Explanation)
	}
	return r, nil
}

// Points returns an offset page of timestamped k6 points, excluding metric definitions.
func Points(path string, offset, limit int) ([]json.RawMessage, bool, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, false, e
	}
	defer f.Close()
	out := []json.RawMessage{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 65536), 1024*1024)
	index := 0
	for s.Scan() {
		var p struct {
			Type string `json:"type"`
		}
		if e = json.Unmarshal(s.Bytes(), &p); e != nil {
			return nil, false, e
		}
		if p.Type != "Point" {
			continue
		}
		if index < offset {
			index++
			continue
		}
		if len(out) == limit {
			return out, true, nil
		}
		out = append(out, append(json.RawMessage(nil), s.Bytes()...))
	}
	return out, false, s.Err()
}
