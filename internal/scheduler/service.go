// Package scheduler turns durable tasks into queued jobs: reminders, delayed
// prompts and maintenance. It polls the tasks table so nothing is lost on
// restart, even with the in-memory queue.
package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Service polls due tasks and enqueues work.
type Service struct {
	tasks ports.TaskRepository
	queue ports.Queue
	cfg   config.SchedulerConfig
	log   *slog.Logger
	stop  chan struct{}
	once  sync.Once
	wg    sync.WaitGroup
}

// New builds the scheduler.
func New(tasks ports.TaskRepository, queue ports.Queue, cfg config.SchedulerConfig, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{tasks: tasks, queue: queue, cfg: cfg, log: log, stop: make(chan struct{})}
}

// Start begins polling.
func (s *Service) Start(ctx context.Context) {
	interval := s.cfg.PollInterval.Std()
	if interval <= 0 {
		interval = 5 * time.Second
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-ticker.C:
				s.dispatchDue(ctx)
			}
		}
	}()
}

// Stop halts polling.
func (s *Service) Stop(context.Context) {
	s.once.Do(func() { close(s.stop) })
	s.wg.Wait()
}

func (s *Service) dispatchDue(ctx context.Context) {
	limit := s.cfg.MaxConcurrent
	if limit <= 0 {
		limit = 5
	}
	due, err := s.tasks.ClaimDue(ctx, time.Now().UTC(), limit)
	if err != nil {
		s.log.Warn("cannot claim due tasks", "error", err)
		return
	}
	for _, task := range due {
		if s.dispatch(ctx, task) {
			_ = s.tasks.Complete(ctx, task.ID, time.Now().UTC())
			continue
		}
		dead := task.Attempts >= task.MaxAttempts
		_ = s.tasks.Fail(ctx, task.ID, "dispatch failed", time.Now().UTC().Add(30*time.Second), dead, time.Now().UTC())
	}
}

func (s *Service) dispatch(ctx context.Context, task domain.Task) bool {
	if s.queue == nil {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		s.log.Warn("invalid task payload", "task_id", task.ID, "kind", task.Kind)
		return false
	}
	payload["task_id"] = task.ID
	raw, _ := json.Marshal(payload)
	kind := task.Kind
	switch kind {
	case "reminder", "automation.prompt", "memory.maintenance", "webhook":
	default:
		kind = "automation.prompt"
	}
	return s.queue.Enqueue(ctx, ports.Job{ID: task.ID, Kind: kind, Payload: raw}, 0) == nil
}
