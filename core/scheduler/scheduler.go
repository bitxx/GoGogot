package scheduler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type Task struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
	entryID   cron.EntryID
}

type TaskInfo struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	Label     string    `json:"label"`
	NextRun   time.Time `json:"next_run"`
	CreatedAt time.Time `json:"created_at"`
}

type Scheduler struct {
	mu    sync.Mutex
	cron  *cron.Cron
	tasks map[string]*Task
	path  string
}

func New(dataDir string) *Scheduler {
	return &Scheduler{
		cron:  cron.New(),
		tasks: make(map[string]*Task),
		path:  filepath.Join(dataDir, "schedules.json"),
	}
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
		slog.Info("scheduler firing task", "id", id, "command", command)

		bin, err := os.Executable()
		if err != nil {
			bin = "gogogot"
		}

		cmd := exec.Command(bin, "--task", command)
		cmd.Env = os.Environ()

		out, err := cmd.CombinedOutput()
		if err != nil {
			slog.Error("scheduled task failed", "id", id, "error", err, "output", string(out))
			return
		}
		slog.Info("scheduled task completed", "id", id, "output_len", len(out))
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
