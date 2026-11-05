package results

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReportSeparates429AndPaginates(t *testing.T) {
	d := t.TempDir()
	summary := `{"metrics":{"successful_latency":{"values":{"count":2,"p(95)":12}},"unsuccessful_requests":{"values":{"rate":0.5},"thresholds":{"rate<=0":{"ok":false}}}}}`
	if e := os.WriteFile(filepath.Join(d, "summary.json"), []byte(summary), 0600); e != nil {
		t.Fatal(e)
	}
	points := `{"type":"Metric","metric":"response_status"}
{"type":"Point","metric":"response_status","data":{"value":1,"tags":{"status":"200"}}}
{"type":"Point","metric":"response_status","data":{"value":1,"tags":{"status":"429"}}}
`
	path := filepath.Join(d, "metrics.json")
	os.WriteFile(path, []byte(points), 0600)
	r, e := Read(d)
	if e != nil {
		t.Fatal(e)
	}
	if r.StatusCounts["200"] != 1 || r.StatusCounts["429"] != 1 || len(r.Insights) != 3 {
		t.Fatalf("unexpected report: %+v", r)
	}
	p, more, e := Points(path, 0, 1)
	if e != nil || len(p) != 1 || !more {
		t.Fatal(p, more, e)
	}
	p, more, e = Points(path, 1, 1)
	if e != nil || len(p) != 1 || more {
		t.Fatal(p, more, e)
	}
}
