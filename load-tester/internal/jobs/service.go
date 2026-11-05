package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/model"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/storage"
	"sync"
	"time"
)

var ErrConflict = errors.New("test is already finished")

type Runner interface {
	Run(context.Context, model.Job) (int, error)
	Cleanup(context.Context, string) error
	Ready(context.Context) error
}
type Service struct {
	Store  *storage.Store
	Runner Runner
	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func New(s *storage.Store, r Runner) *Service {
	return &Service{Store: s, Runner: r, active: map[string]context.CancelFunc{}}
}
func (s *Service) Create(c model.Config) (model.Job, error) {
	if e := c.Validate(); e != nil {
		return model.Job{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, e := s.Store.Pending()
	if e != nil {
		return model.Job{}, e
	}
	if len(pending) >= 20 {
		return model.Job{}, fmt.Errorf("queue is full")
	}
	var b [16]byte
	if _, e = rand.Read(b[:]); e != nil {
		return model.Job{}, e
	}
	j := model.Job{ID: hex.EncodeToString(b[:]), Config: c, Status: "queued", CreatedAt: time.Now().UTC()}
	return j, s.Store.Save(j)
}
func (s *Service) Cancel(id string) (model.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, e := s.Store.Get(id)
	if e != nil {
		return j, e
	}
	if j.Status != "queued" && j.Status != "running" {
		return j, ErrConflict
	}
	now := time.Now().UTC()
	if j.Status == "running" {
		j.Status = "cancelling"
	} else {
		j.Status = "cancelled"
		j.FinishedAt = &now
	}
	if e = s.Store.Save(j); e != nil {
		return j, e
	}
	if cancel := s.active[id]; cancel != nil {
		cancel()
	}
	return j, nil
}

// Recover runs before serving requests. Previously running tests are interrupted,
// not silently rerun; queued tests remain durable and will be picked up.
func (s *Service) Recover(ctx context.Context) error {
	jobs, e := s.Store.Pending()
	if e != nil {
		return e
	}
	for _, j := range jobs {
		if j.Status != "running" && j.Status != "cancelling" {
			continue
		}
		if e = s.Runner.Cleanup(ctx, j.ID); e != nil {
			return e
		}
		now := time.Now().UTC()
		if j.Status == "cancelling" {
			j.Status = "cancelled"
		} else {
			j.Status = "failed"
			j.Error = "service restarted during execution"
		}
		j.FinishedAt = &now
		if e = s.Store.Save(j); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) Work(ctx context.Context) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		s.mu.Lock()
		pending, e := s.Store.Pending()
		if e != nil {
			s.mu.Unlock()
			return e
		}
		var next *model.Job
		for i := range pending {
			if pending[i].Status == "queued" {
				next = &pending[i]
				break
			}
		}
		if next == nil {
			s.mu.Unlock()
			continue
		}
		j := *next
		now := time.Now().UTC()
		j.Status = "running"
		j.StartedAt = &now
		runCtx, cancel := context.WithTimeout(ctx, time.Duration(j.Config.TotalDurationSeconds()+j.Config.RequestTimeoutSeconds+180)*time.Second)
		if e = s.Store.Save(j); e != nil {
			cancel()
			s.mu.Unlock()
			return e
		}
		s.active[j.ID] = cancel
		s.mu.Unlock()
		code, runErr := s.Runner.Run(runCtx, j)
		cancel()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		cleanupErr := s.Runner.Cleanup(cleanupCtx, j.ID)
		cleanupCancel()
		if cleanupErr != nil {
			return fmt.Errorf("cleanup incomplete for %s; restart to reconcile: %w", j.ID, cleanupErr)
		}
		s.mu.Lock()
		delete(s.active, j.ID)
		current, e := s.Store.Get(j.ID)
		if e == nil && current.Status != "cancelled" {
			end := time.Now().UTC()
			current.FinishedAt = &end
			current.ExitCode = &code
			if current.Status == "cancelling" {
				current.Status = "cancelled"
			} else if runErr != nil {
				current.Status = "failed"
				current.Error = runErr.Error()
			} else if code == 0 || code == 99 {
				current.Status = "completed"
				passed := code == 0
				current.ThresholdsPassed = &passed
			} else {
				current.Status = "failed"
				current.Error = fmt.Sprintf("k6 exited with code %d", code)
			}
			e = s.Store.Save(current)
		}
		s.mu.Unlock()
		if e != nil {
			return e
		}
	}
}
