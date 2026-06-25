package teamtask

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"PseudoClaude/internal/team/filelock"
)

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusBlocked    Status = "blocked"
)

type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Assignee    string    `json:"assignee,omitempty"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	BlockedBy   []string  `json:"blocked_by,omitempty"`
	Blocks      []string  `json:"blocks,omitempty"`
}

type ListedTask struct {
	Task
	IsReady bool `json:"is_ready"`
}

type CreateInput struct {
	Title       string
	Description string
	Assignee    string
	Status      Status
}

type ListFilter struct {
	Status Status
}

type Patch struct {
	Title           *string
	Description     *string
	Assignee        *string
	Status          *Status
	AddBlockedBy    []string
	RemoveBlockedBy []string
	AddBlocks       []string
	RemoveBlocks    []string
}

type Store struct {
	path string
}

type state struct {
	Tasks []Task `json:"tasks"`
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Create(ctx context.Context, in CreateInput) (Task, error) {
	now := time.Now()
	task := Task{
		ID:          "task_" + randomHex(6),
		Title:       in.Title,
		Description: in.Description,
		Assignee:    in.Assignee,
		Status:      in.Status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if task.Status == "" {
		task.Status = StatusTodo
	}
	err := s.withLock(ctx, func(st *state) error {
		st.Tasks = append(st.Tasks, task)
		return nil
	})
	return task, err
}

func (s *Store) Get(id string) (Task, bool, error) {
	st, err := s.read()
	if err != nil {
		return Task{}, false, err
	}
	for _, task := range st.Tasks {
		if task.ID == id {
			return task, true, nil
		}
	}
	return Task{}, false, nil
}

func (s *Store) List(filter ListFilter) ([]ListedTask, error) {
	st, err := s.read()
	if err != nil {
		return nil, err
	}
	byID := map[string]Task{}
	for _, task := range st.Tasks {
		byID[task.ID] = task
	}
	out := []ListedTask{}
	for _, task := range st.Tasks {
		if filter.Status != "" && task.Status != filter.Status {
			continue
		}
		out = append(out, ListedTask{Task: task, IsReady: isReady(task, byID)})
	}
	return out, nil
}

func (s *Store) Update(ctx context.Context, id string, patch Patch) (Task, error) {
	var updated Task
	err := s.withLock(ctx, func(st *state) error {
		idx := slices.IndexFunc(st.Tasks, func(task Task) bool { return task.ID == id })
		if idx < 0 {
			return fmt.Errorf("task %q not found", id)
		}
		task := st.Tasks[idx]
		if patch.Title != nil {
			task.Title = *patch.Title
		}
		if patch.Description != nil {
			task.Description = *patch.Description
		}
		if patch.Assignee != nil {
			task.Assignee = *patch.Assignee
		}
		if patch.Status != nil {
			task.Status = *patch.Status
		}
		for _, dep := range patch.AddBlockedBy {
			task.BlockedBy = addUnique(task.BlockedBy, dep)
			if depIdx := indexTask(st.Tasks, dep); depIdx >= 0 {
				st.Tasks[depIdx].Blocks = addUnique(st.Tasks[depIdx].Blocks, id)
			}
		}
		for _, dep := range patch.RemoveBlockedBy {
			task.BlockedBy = removeValue(task.BlockedBy, dep)
			if depIdx := indexTask(st.Tasks, dep); depIdx >= 0 {
				st.Tasks[depIdx].Blocks = removeValue(st.Tasks[depIdx].Blocks, id)
			}
		}
		for _, target := range patch.AddBlocks {
			task.Blocks = addUnique(task.Blocks, target)
			if targetIdx := indexTask(st.Tasks, target); targetIdx >= 0 {
				st.Tasks[targetIdx].BlockedBy = addUnique(st.Tasks[targetIdx].BlockedBy, id)
			}
		}
		for _, target := range patch.RemoveBlocks {
			task.Blocks = removeValue(task.Blocks, target)
			if targetIdx := indexTask(st.Tasks, target); targetIdx >= 0 {
				st.Tasks[targetIdx].BlockedBy = removeValue(st.Tasks[targetIdx].BlockedBy, id)
			}
		}
		task.UpdatedAt = time.Now()
		st.Tasks[idx] = task
		updated = task
		return nil
	})
	return updated, err
}

func (s *Store) withLock(ctx context.Context, mutate func(*state) error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	release, err := filelock.Acquire(ctx, s.path+".lock", filelock.Options{})
	if err != nil {
		return err
	}
	defer release()
	st, err := s.read()
	if err != nil {
		return err
	}
	if err := mutate(st); err != nil {
		return err
	}
	return atomicWriteJSON(s.path, st)
}

func (s *Store) read() (*state, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &state{}, nil
		}
		return nil, err
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func isReady(task Task, byID map[string]Task) bool {
	for _, id := range task.BlockedBy {
		if byID[id].Status != StatusDone {
			return false
		}
	}
	return true
}

func indexTask(tasks []Task, id string) int {
	return slices.IndexFunc(tasks, func(task Task) bool { return task.ID == id })
}

func addUnique(values []string, value string) []string {
	if value == "" || slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func removeValue(values []string, value string) []string {
	return slices.DeleteFunc(values, func(v string) bool { return v == value })
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return hex.EncodeToString(buf)
}

func atomicWriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}
