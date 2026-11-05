package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/api"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/jobs"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/model"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/storage"
)

type fakeRunner struct {
	code    int
	wait    bool
	started chan struct{}
}

func (f *fakeRunner) Ready(context.Context) error           { return nil }
func (f *fakeRunner) Cleanup(context.Context, string) error { return nil }
func (f *fakeRunner) Run(ctx context.Context, j model.Job) (int, error) {
	if f.started != nil {
		close(f.started)
	}
	if f.wait {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	return f.code, nil
}
func fixture(t *testing.T, r *fakeRunner) (*jobs.Service, http.Handler) {
	t.Helper()
	s, e := storage.Open(filepath.Join(t.TempDir(), "jobs.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	j := jobs.New(s, r)
	return j, (&api.API{Jobs: j, DataDir: t.TempDir()}).Handler()
}
func request(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
	return w
}

const valid = `{"targetUrl":"http://example.test/api?token=secret","virtualUsers":1,"durationSeconds":1,"headers":{"Authorization":"secret"},"body":"secret"}`

func submit(t *testing.T, h http.Handler) model.Job {
	t.Helper()
	w := request(h, "POST", "/tests", valid)
	if w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if bytes.Contains(w.Body.Bytes(), []byte("secret")) {
		t.Fatal("configuration secrets exposed")
	}
	var j model.Job
	if e := json.Unmarshal(w.Body.Bytes(), &j); e != nil {
		t.Fatal(e)
	}
	return j
}
func work(t *testing.T, s *jobs.Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Work(ctx) }()
	t.Cleanup(func() {
		cancel()
		if e := <-done; e != nil {
			t.Error(e)
		}
	})
}
func await(t *testing.T, s *jobs.Service, id, status string) model.Job {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j, e := s.Store.Get(id)
		if e != nil {
			t.Fatal(e)
		}
		if j.Status == status {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job did not reach %s", status)
	return model.Job{}
}
func TestThresholdFailureCompletes(t *testing.T) {
	s, h := fixture(t, &fakeRunner{code: 99})
	j := submit(t, h)
	work(t, s)
	j = await(t, s, j.ID, "completed")
	if j.ThresholdsPassed == nil || *j.ThresholdsPassed || *j.ExitCode != 99 {
		t.Fatalf("wrong outcome: %+v", j)
	}
}
func TestCancelRunning(t *testing.T) {
	r := &fakeRunner{wait: true, started: make(chan struct{})}
	s, h := fixture(t, r)
	j := submit(t, h)
	work(t, s)
	select {
	case <-r.started:
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not start")
	}
	w := request(h, "POST", "/tests/"+j.ID+"/cancel", "")
	if w.Code != 202 {
		t.Fatal(w.Body)
	}
	await(t, s, j.ID, "cancelled")
	if w = request(h, "POST", "/tests/"+j.ID+"/cancel", ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestQueuedCancellationAndValidation(t *testing.T) {
	s, h := fixture(t, &fakeRunner{})
	j := submit(t, h)
	w := request(h, "GET", "/tests/"+j.ID+"/results", "")
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	w = request(h, "POST", "/tests/"+j.ID+"/cancel", "")
	if w.Code != 202 {
		t.Fatal(w.Body)
	}
	await(t, s, j.ID, "cancelled")
	for _, body := range []string{`{"targetUrl":"file:///etc/passwd"}`, valid + ` {}`, `{"unknown":true}`} {
		if w = request(h, "POST", "/tests", body); w.Code != 400 {
			t.Fatalf("accepted %s", body)
		}
	}
	if w = request(h, "GET", "/tests?limit=-1", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = request(h, "GET", "/tests/../../etc/passwd", ""); w.Code == 200 {
		t.Fatal("invalid id accepted")
	}
}
func TestDurabilityAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.sqlite")
	s, e := storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	service := jobs.New(s, &fakeRunner{})
	c := model.Config{TargetURL: "http://example.test", VirtualUsers: 1, DurationSeconds: 1}
	queued, e := service.Create(c)
	if e != nil {
		t.Fatal(e)
	}
	running, e := service.Create(c)
	if e != nil {
		t.Fatal(e)
	}
	running.Status = "running"
	if e = s.Save(running); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	service = jobs.New(s, &fakeRunner{})
	if e = service.Recover(context.Background()); e != nil {
		t.Fatal(e)
	}
	await(t, service, queued.ID, "queued")
	j := await(t, service, running.ID, "failed")
	if j.Error == "" {
		t.Fatal("missing interruption reason")
	}
}

func TestRequestedRateAPI(t *testing.T) {
	s, h := fixture(t, &fakeRunner{})
	body := `{"targetUrl":"http://example.test","requestsPerSecond":10,"durationSeconds":30,"p95ThresholdMs":200}`
	w := request(h, "POST", "/tests", body)
	if w.Code != 202 {
		t.Fatalf("%d: %s", w.Code, w.Body)
	}
	var j model.Job
	if e := json.Unmarshal(w.Body.Bytes(), &j); e != nil {
		t.Fatal(e)
	}
	stored, e := s.Store.Get(j.ID)
	if e != nil {
		t.Fatal(e)
	}
	if stored.Config.RequestsPerSecond != 10 || stored.Config.MaxVirtualUsers != 50 || stored.Config.HeadroomPercent != 20 {
		t.Fatalf("unexpected persisted workload: %+v", stored.Config)
	}
	for _, body := range []string{
		`{"targetUrl":"http://example.test","requestsPerSecond":1001,"durationSeconds":30}`,
		`{"targetUrl":"http://example.test","rateSteps":[{"requestsPerSecond":10,"durationSeconds":30}],"p95ThresholdMs":0}`,
	} {
		if w = request(h, "POST", "/tests", body); w.Code != 400 {
			t.Fatalf("accepted invalid rate config: %s", w.Body)
		}
	}
}
