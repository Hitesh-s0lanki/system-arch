package model

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type RateStep struct {
	RequestsPerSecond int `json:"requestsPerSecond"`
	DurationSeconds   int `json:"durationSeconds"`
}

type Config struct {
	RequestsPerSecond     int               `json:"requestsPerSecond,omitempty"`
	RateSteps             []RateStep        `json:"rateSteps,omitempty"`
	MaxVirtualUsers       int               `json:"maxVirtualUsers,omitempty"`
	HeadroomPercent       int               `json:"headroomPercent,omitempty"`
	TargetURL             string            `json:"targetUrl"`
	Method                string            `json:"method"`
	Headers               map[string]string `json:"headers,omitempty"`
	Body                  string            `json:"body,omitempty"`
	VirtualUsers          int               `json:"virtualUsers"`
	DurationSeconds       int               `json:"durationSeconds"`
	RequestTimeoutSeconds int               `json:"requestTimeoutSeconds"`
	P95ThresholdMS        float64           `json:"p95ThresholdMs,omitempty"`
	MaxErrorRate          float64           `json:"maxErrorRate"`
}

func (c *Config) Validate() error {
	u, e := url.Parse(c.TargetURL)
	if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("targetUrl must be an HTTP(S) URL without credentials or fragment")
	}
	if len(c.TargetURL) > 4096 {
		return fmt.Errorf("targetUrl is too long")
	}
	if c.Method == "" {
		c.Method = "GET"
	}
	c.Method = strings.ToUpper(c.Method)
	switch c.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		return fmt.Errorf("unsupported HTTP method")
	}
	if c.RequestTimeoutSeconds == 0 {
		c.RequestTimeoutSeconds = 10
	}
	if c.RequestTimeoutSeconds < 1 || c.RequestTimeoutSeconds > 30 {
		return fmt.Errorf("requestTimeoutSeconds must be 1–30")
	}
	if c.RequestsPerSecond < 0 || c.RequestsPerSecond > 1000 {
		return fmt.Errorf("requestsPerSecond must be 1–1000 when supplied")
	}
	if c.RequestsPerSecond > 0 && len(c.RateSteps) > 0 {
		return fmt.Errorf("use requestsPerSecond or rateSteps, not both")
	}
	if c.ArrivalRate() {
		if c.VirtualUsers == 0 {
			c.VirtualUsers = 10
		}
		if c.MaxVirtualUsers == 0 {
			c.MaxVirtualUsers = max(50, c.VirtualUsers)
		}
		if c.MaxVirtualUsers < c.VirtualUsers || c.MaxVirtualUsers > 200 {
			return fmt.Errorf("maxVirtualUsers must be between virtualUsers and 200")
		}
		if c.HeadroomPercent == 0 {
			c.HeadroomPercent = 20
		}
		if c.HeadroomPercent < 1 || c.HeadroomPercent > 90 {
			return fmt.Errorf("headroomPercent must be 1–90")
		}
	} else if c.MaxVirtualUsers != 0 || c.HeadroomPercent != 0 {
		return fmt.Errorf("maxVirtualUsers and headroomPercent require an arrival-rate workload")
	}
	if c.VirtualUsers < 1 || c.VirtualUsers > 200 {
		return fmt.Errorf("virtualUsers must be 1–200")
	}
	if len(c.RateSteps) > 0 {
		if len(c.RateSteps) > 8 || c.DurationSeconds != 0 {
			return fmt.Errorf("rateSteps allows at most 8 steps; omit top-level durationSeconds")
		}
		if c.P95ThresholdMS <= 0 {
			return fmt.Errorf("p95ThresholdMs must be positive for capacity exploration")
		}
		previous := 0
		for _, step := range c.RateSteps {
			if step.RequestsPerSecond <= previous || step.RequestsPerSecond > 1000 || step.DurationSeconds < 1 || step.DurationSeconds > 300 {
				return fmt.Errorf("rateSteps must have increasing rates (1–1000) and durations (1–300 seconds)")
			}
			previous = step.RequestsPerSecond
		}
		if c.TotalDurationSeconds()+c.RequestTimeoutSeconds > 300 {
			return fmt.Errorf("rateSteps including graceful drain time must fit within 300 seconds")
		}
	} else if c.DurationSeconds < 1 || c.DurationSeconds > 300 {
		return fmt.Errorf("durationSeconds must be 1–300")
	}
	if c.P95ThresholdMS < 0 || c.MaxErrorRate < 0 || c.MaxErrorRate > 1 {
		return fmt.Errorf("invalid latency threshold or maxErrorRate (0–1)")
	}
	if len(c.Body) > 65536 || len(c.Headers) > 32 {
		return fmt.Errorf("body or headers exceed limits")
	}
	for k, v := range c.Headers {
		if len(k) > 128 || len(v) > 8192 || strings.ContainsAny(k+v, "\r\n") {
			return fmt.Errorf("invalid header")
		}
	}
	return nil
}

func (c Config) ArrivalRate() bool { return c.RequestsPerSecond > 0 || len(c.RateSteps) > 0 }
func (c Config) Steps() []RateStep {
	if len(c.RateSteps) > 0 {
		return c.RateSteps
	}
	if c.RequestsPerSecond > 0 {
		return []RateStep{{RequestsPerSecond: c.RequestsPerSecond, DurationSeconds: c.DurationSeconds}}
	}
	return nil
}
func (c Config) TotalDurationSeconds() int {
	if len(c.RateSteps) == 0 {
		return c.DurationSeconds
	}
	total := (len(c.RateSteps) - 1) * c.RequestTimeoutSeconds
	for _, step := range c.RateSteps {
		total += step.DurationSeconds
	}
	return total
}
func (c Config) Scenario(index int) string {
	if len(c.RateSteps) == 0 {
		return "benchmark"
	}
	return fmt.Sprintf("step_%d", index+1)
}

type Job struct {
	ID               string     `json:"id"`
	Status           string     `json:"status"`
	Config           Config     `json:"config"`
	CreatedAt        time.Time  `json:"createdAt"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	Error            string     `json:"error,omitempty"`
	ExitCode         *int       `json:"exitCode,omitempty"`
	ThresholdsPassed *bool      `json:"thresholdsPassed,omitempty"`
}

func (j Job) Public() Job {
	if u, e := url.Parse(j.Config.TargetURL); e == nil {
		u.RawQuery = ""
		j.Config.TargetURL = u.String()
	}
	j.Config.Headers = nil
	j.Config.Body = ""
	return j
}
