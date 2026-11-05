package model

import "testing"

func TestArrivalValidation(t *testing.T) {
	base := Config{TargetURL: "http://example.test", RequestsPerSecond: 10, DurationSeconds: 30, P95ThresholdMS: 200}
	if e := base.Validate(); e != nil {
		t.Fatal(e)
	}
	if base.VirtualUsers != 10 || base.MaxVirtualUsers != 50 || base.HeadroomPercent != 20 {
		t.Fatalf("defaults: %+v", base)
	}
	if e := base.Validate(); e != nil {
		t.Fatalf("validation must be idempotent: %v", e)
	}
	stepped := Config{TargetURL: "http://example.test", RateSteps: []RateStep{{5, 30}, {10, 30}}, P95ThresholdMS: 200, RequestTimeoutSeconds: 2}
	if e := stepped.Validate(); e != nil {
		t.Fatal(e)
	}
	if stepped.TotalDurationSeconds() != 62 || stepped.Scenario(1) != "step_2" {
		t.Fatal(stepped)
	}
	for name, change := range map[string]func(*Config){
		"mixed modes":       func(c *Config) { c.RequestsPerSecond = 1 },
		"descending":        func(c *Config) { c.RateSteps = []RateStep{{10, 30}, {5, 30}} },
		"duplicate":         func(c *Config) { c.RateSteps = []RateStep{{5, 30}, {5, 30}} },
		"unbounded rate":    func(c *Config) { c.RateSteps = []RateStep{{1001, 30}} },
		"deadline":          func(c *Config) { c.RateSteps = []RateStep{{5, 150}, {10, 150}} },
		"no latency target": func(c *Config) { c.P95ThresholdMS = 0 },
		"top duration":      func(c *Config) { c.DurationSeconds = 30 },
		"invalid VU cap":    func(c *Config) { c.MaxVirtualUsers = 1 },
		"headroom":          func(c *Config) { c.HeadroomPercent = 100 },
	} {
		t.Run(name, func(t *testing.T) {
			c := stepped
			change(&c)
			if e := c.Validate(); e == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}
