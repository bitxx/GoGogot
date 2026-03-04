package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
)

// TaskExecutor runs a scheduled command in-process and returns the agent's
// text output. The context carries a timeout; implementations must respect it.
type TaskExecutor func(ctx context.Context, taskID, command string) (string, error)

// backoffSchedule defines exponential delays indexed by consecutive error count.
var backoffSchedule = []time.Duration{
	30 * time.Second,  // 1st error
	1 * time.Minute,   // 2nd error
	5 * time.Minute,   // 3rd error
	15 * time.Minute,  // 4th error
	60 * time.Minute,  // 5th+ error
}

const (
	defaultTaskTimeout  = 5 * time.Minute
	maxConcurrentTasks  = 2
)

type TaskState struct {
	LastRunAt         time.Time `json:"last_run_at,omitempty"`
	LastStatus        string    `json:"last_status,omitempty"`
	LastError         string    `json:"last_error,omitempty"`
	LastDurationMs    int64     `json:"last_duration_ms,omitempty"`
	ConsecutiveErrors int       `json:"consecutive_errors,omitempty"`
}

type Task struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
	State     TaskState `json:"state"`

	entryID cron.EntryID
	running atomic.Bool
}

type TaskInfo struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	Label     string    `json:"label"`
	NextRun   time.Time `json:"next_run"`
	CreatedAt time.Time `json:"created_at"`
	State     TaskState `json:"state"`
}

type Scheduler struct {
	mu       sync.Mutex
	cron     *cron.Cron
	tasks    map[string]*Task
	path     string
	executor TaskExecutor
	sem      chan struct{}
}

func New(dataDir string, executor TaskExecutor) *Scheduler {
	return &Scheduler{
		cron:     cron.New(),
		tasks:    make(map[string]*Task),
		path:     filepath.Join(dataDir, "schedules.json"),
		executor: executor,
		sem:      make(chan struct{}, maxConcurrentTasks),
	}
}

func (s *Scheduler) SetExecutor(exec TaskExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = exec
}

func (s *Scheduler) Start() error {
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to load schedules", "error", err)
	}
	s.cron.Start()
	slog.Info("scheduler started", "tasks", len(s.tasks))
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *Scheduler) Add(id, schedule, command, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.tasks[id]; ok {
		s.cron.Remove(existing.entryID)
	}

	entryID, err := s.cron.AddFunc(schedule, s.makeRunner(id, command))
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", schedule, err)
	}

	s.tasks[id] = &Task{
		ID:        id,
		Schedule:  schedule,
		Command:   command,
		Label:     label,
		CreatedAt: time.Now(),
		entryID:   entryID,
	}

	return s.save()
}

func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}

	s.cron.Remove(task.entryID)
	delete(s.tasks, id)
	return s.save()
}

func (s *Scheduler) List() []TaskInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]TaskInfo, 0, len(s.tasks))
	for _, t := range s.tasks {
		info := TaskInfo{
			ID:        t.ID,
			Schedule:  t.Schedule,
			Command:   t.Command,
			Label:     t.Label,
			CreatedAt: t.CreatedAt,
			State:     t.State,
		}
		if entry := s.cron.Entry(t.entryID); !entry.Next.IsZero() {
			info.NextRun = entry.Next
		}
		out = append(out, info)
	}
	return out
}

func (s *Scheduler) makeRunner(id, command string) func() {
	return func() {
		s.mu.Lock()
		task, ok := s.tasks[id]
		s.mu.Unlock()
		if !ok {
			return
		}

		// Singleton guard: skip if already running.
		if !task.running.CompareAndSwap(false, true) {
			slog.Warn("scheduler: task already running, skipping", "id", id)
			return
		}
		defer task.running.Store(false)

		// Exponential backoff: skip if too soon after consecutive errors.
		if task.State.ConsecutiveErrors > 0 && !task.State.LastRunAt.IsZero() {
			idx := task.State.ConsecutiveErrors - 1
			if idx >= len(backoffSchedule) {
				idx = len(backoffSchedule) - 1
			}
			cooldown := backoffSchedule[idx]
			if time.Since(task.State.LastRunAt) < cooldown {
				slog.Info("scheduler: backoff active, skipping",
					"id", id,
					"consecutive_errors", task.State.ConsecutiveErrors,
					"cooldown", cooldown,
				)
				return
			}
		}

		// Acquire concurrency semaphore.
		s.sem <- struct{}{}
		defer func() { <-s.sem }()

		slog.Info("scheduler firing task", "id", id, "command", command)
		start := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), defaultTaskTimeout)
		defer cancel()

		output, err := s.executor(ctx, id, command)

		elapsed := time.Since(start)
		state := TaskState{
			LastRunAt:      start,
			LastDurationMs: elapsed.Milliseconds(),
		}

		if err != nil {
			state.LastStatus = "error"
			state.LastError = err.Error()
			state.ConsecutiveErrors = task.State.ConsecutiveErrors + 1
			slog.Error("scheduled task failed",
				"id", id,
				"error", err,
				"consecutive_errors", state.ConsecutiveErrors,
				"duration", elapsed,
			)
		} else {
			state.LastStatus = "ok"
			state.ConsecutiveErrors = 0
			slog.Info("scheduled task completed",
				"id", id,
				"output_len", len(output),
				"duration", elapsed,
			)
		}

		s.mu.Lock()
		if t, ok := s.tasks[id]; ok {
			t.State = state
			_ = s.save()
		}
		s.mu.Unlock()
	}
}

func (s *Scheduler) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return err
	}

	for _, t := range tasks {
		entryID, err := s.cron.AddFunc(t.Schedule, s.makeRunner(t.ID, t.Command))
		if err != nil {
			slog.Error("failed to restore scheduled task", "id", t.ID, "schedule", t.Schedule, "error", err)
			continue
		}
		t.entryID = entryID
		s.tasks[t.ID] = t
	}

	slog.Info("loaded schedules from disk", "count", len(s.tasks))
	return nil
}

func (s *Scheduler) save() error {
	tasks := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
