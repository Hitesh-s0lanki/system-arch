package results

import (
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/model"
	"testing"
)

func rateFixture() (Report, model.Config) {
	c := model.Config{RateSteps: []model.RateStep{{RequestsPerSecond: 10, DurationSeconds: 30}, {RequestsPerSecond: 20, DurationSeconds: 30}}, P95ThresholdMS: 100, MaxErrorRate: 0.01, HeadroomPercent: 20}
	r := Report{Metrics: map[string]Metric{}, scenarioCounts: map[string]map[string]int64{"step_1": {"200": 300}, "step_2": {"200": 600}}}
	for _, name := range []string{"step_1", "step_2"} {
		r.Metrics["successful_latency{scenario:"+name+"}"] = Metric{Values: map[string]float64{"p(95)": 50}}
	}
	return r, c
}
func TestRateSuggestionAndBoundary(t *testing.T) {
	r, c := rateFixture()
	a := analyzeRates(r, c, true)
	if a.HighestPassingRPS != 20 || a.RecommendedRateLimitRPS == nil || *a.RecommendedRateLimitRPS != 16 || a.Boundary != "lower_bound" {
		t.Fatalf("%+v", a)
	}
	r.scenarioCounts["step_2"] = map[string]int64{"200": 500, "429": 100}
	a = analyzeRates(r, c, true)
	if a.HighestPassingRPS != 10 || *a.RecommendedRateLimitRPS != 8 || a.Boundary != "threshold_crossed" || a.Steps[1].Status != "failed" {
		t.Fatalf("%+v", a)
	}
}
func TestNeverRecommendFromInsufficientEvidence(t *testing.T) {
	for name, change := range map[string]func(*Report, *model.Config){
		"too short":    func(r *Report, c *model.Config) { c.RateSteps[0].DurationSeconds = 5 },
		"no successes": func(r *Report, c *model.Config) { r.scenarioCounts["step_1"] = map[string]int64{"500": 300} },
		"latency violation": func(r *Report, c *model.Config) {
			r.Metrics["successful_latency{scenario:step_1}"] = Metric{Values: map[string]float64{"p(95)": 200}}
		},
		"dropped load": func(r *Report, c *model.Config) {
			r.Metrics["dropped_iterations{scenario:step_1}"] = Metric{Values: map[string]float64{"count": 3}}
		},
		"underdelivered":     func(r *Report, c *model.Config) { r.scenarioCounts["step_1"] = map[string]int64{"200": 250} },
		"missing percentile": func(r *Report, c *model.Config) { delete(r.Metrics, "successful_latency{scenario:step_1}") },
		"too few samples": func(r *Report, c *model.Config) {
			c.RateSteps[0].RequestsPerSecond = 2
			r.scenarioCounts["step_1"] = map[string]int64{"200": 60}
		},
		"missing latency target": func(r *Report, c *model.Config) { c.P95ThresholdMS = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			r, c := rateFixture()
			change(&r, &c)
			a := analyzeRates(r, c, true)
			if a.RecommendedRateLimitRPS != nil || a.HighestPassingRPS != 0 {
				t.Fatalf("unsafe suggestion: %+v", a)
			}
		})
	}
	r, c := rateFixture()
	if a := analyzeRates(r, c, false); a.RecommendedRateLimitRPS != nil {
		t.Fatal("interrupted run produced recommendation")
	}
}
func TestFixedRPSAnalysis(t *testing.T) {
	c := model.Config{RequestsPerSecond: 10, DurationSeconds: 30, P95ThresholdMS: 100, HeadroomPercent: 20}
	r := Report{Metrics: map[string]Metric{"successful_latency{scenario:benchmark}": {Values: map[string]float64{"p(95)": 50}}}, scenarioCounts: map[string]map[string]int64{"benchmark": {"200": 300}}}
	a := analyzeRates(r, c, true)
	if a.HighestPassingRPS != 10 || *a.RecommendedRateLimitRPS != 8 {
		t.Fatal(a)
	}
}
