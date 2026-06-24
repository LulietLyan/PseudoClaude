package task

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/conversation"
)

type Manager struct {
	mu          sync.RWMutex
	tasks       map[string]*BackgroundTask
	byName      map[string]string
	done        chan DoneEvent
	idSource    IDSource
	run         RunFunc
	autoTimeout time.Duration
}

func NewManager(opts Options) *Manager {
	idSource := opts.IDSource
	if idSource == nil {
		idSource = defaultIDSource()
	}
	run := opts.Run
	if run == nil {
		run = defaultRun
	}
	return &Manager{
		tasks:       map[string]*BackgroundTask{},
		byName:      map[string]string{},
		done:        make(chan DoneEvent, 64),
		idSource:    idSource,
		run:         run,
		autoTimeout: opts.AutoTimeout,
	}
}

func (m *Manager) Launch(ctx context.Context, in LaunchInput) (string, error) {
	if m == nil {
		return "", fmt.Errorf("task manager is nil")
	}
	if in.Conversation == nil {
		in.Conversation = &conversation.Conversation{}
	}
	taskCtx, cancel := context.WithCancel(context.Background())
	id := m.idSource()
	task := &BackgroundTask{
		ID:           id,
		Name:         in.Name,
		Type:         in.Type,
		Fork:         in.Fork,
		Status:       StatusRunning,
		Prompt:       in.Prompt,
		StartedAt:    time.Now(),
		Cancel:       cancel,
		Runner:       in.Runner,
		Conversation: in.Conversation,
		LastActivity: "started",
	}
	m.mu.Lock()
	if in.Name != "" {
		if existing, ok := m.byName[in.Name]; ok {
			if old := m.tasks[existing]; old != nil && old.Status == StatusRunning {
				m.mu.Unlock()
				cancel()
				return "", fmt.Errorf("task name %q is already running", in.Name)
			}
		}
		m.byName[in.Name] = id
	}
	m.tasks[id] = task
	m.mu.Unlock()
	go m.runTask(taskCtx, id, in.Prompt, in.Prepare)
	return id, nil
}

func (m *Manager) LaunchAgent(ctx context.Context, in agent.AgentLaunchInput) (string, error) {
	return m.Launch(ctx, LaunchInput{
		Name:         in.Name,
		Type:         in.Type,
		Fork:         in.Fork,
		Prompt:       in.Prompt,
		Runner:       in.Runner,
		Conversation: in.Conversation,
		Prepare:      in.Prepare,
	})
}

func (m *Manager) Adopt(ctx context.Context, in AdoptInput) (string, error) {
	return m.Launch(ctx, in.LaunchInput)
}

func (m *Manager) List() []Snapshot {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Snapshot, 0, len(m.tasks))
	for _, task := range m.tasks {
		out = append(out, snapshotOf(task))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out
}

func (m *Manager) Get(id string) (Snapshot, bool) {
	if m == nil {
		return Snapshot{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	return snapshotOf(task), ok
}

func (m *Manager) Stop(id string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return false
	}
	if task.Status != StatusRunning {
		m.mu.Unlock()
		return true
	}
	task.Status = StatusCancelled
	task.EndedAt = time.Now()
	task.LastActivity = "cancel requested"
	cancel := task.Cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.publishDone(id)
	return true
}

func (m *Manager) SendMessage(ctx context.Context, name, message string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("task manager is nil")
	}
	m.mu.RLock()
	id, ok := m.byName[name]
	task := m.tasks[id]
	if !ok || task == nil {
		m.mu.RUnlock()
		return "", fmt.Errorf("task name %q not found", name)
	}
	if task.Status == StatusRunning {
		m.mu.RUnlock()
		return "", fmt.Errorf("task name %q is still running", name)
	}
	if task.Status == StatusCancelled || task.Status == StatusFailed {
		m.mu.RUnlock()
		return "", fmt.Errorf("task name %q cannot be continued from status %s", name, task.Status)
	}
	in := LaunchInput{
		Name:         name,
		Type:         task.Type,
		Fork:         task.Fork,
		Prompt:       message,
		Runner:       task.Runner,
		Conversation: task.Conversation,
	}
	m.mu.RUnlock()
	return m.Launch(ctx, in)
}

func (m *Manager) SubscribeDone() <-chan DoneEvent {
	if m == nil {
		ch := make(chan DoneEvent)
		close(ch)
		return ch
	}
	return m.done
}

func (m *Manager) runTask(ctx context.Context, id, prompt string, prepare agent.AgentPrepareFunc) {
	defer func() {
		if recovered := recover(); recovered != nil {
			m.finish(id, StatusFailed, "", fmt.Sprintf("panic: %v", recovered), agent.CompletionResult{})
		}
	}()
	m.mu.RLock()
	task := m.tasks[id]
	if task == nil {
		m.mu.RUnlock()
		return
	}
	runner := task.Runner
	conv := task.Conversation
	m.mu.RUnlock()
	var cleanup agent.AgentCleanupFunc
	if prepare != nil {
		nextRunner, nextPrompt, nextCleanup, err := prepare(ctx, runner, prompt)
		if err != nil {
			m.finish(id, StatusFailed, "", err.Error(), agent.CompletionResult{Stop: agent.Stop{Reason: agent.StopStreamError, Message: err.Error()}})
			return
		}
		runner = nextRunner
		prompt = nextPrompt
		cleanup = nextCleanup
		m.mu.Lock()
		if task := m.tasks[id]; task != nil {
			task.Runner = runner
			task.Prompt = prompt
			task.LastActivity = "prepared"
		}
		m.mu.Unlock()
	}
	result := m.run(ctx, runner, conv, prompt)
	if cleanup != nil {
		result.Text = cleanup(context.Background(), result.Text)
	}
	status := statusFromStop(result.Stop.Reason)
	errText := ""
	if status == StatusFailed {
		errText = result.Stop.Message
	}
	if status == StatusCancelled && result.Stop.Message != "" {
		errText = result.Stop.Message
	}
	m.finish(id, status, result.Text, errText, result)
}

func (m *Manager) finish(id string, status Status, resultText, errText string, result agent.CompletionResult) {
	m.mu.Lock()
	task := m.tasks[id]
	if task == nil {
		m.mu.Unlock()
		return
	}
	if task.Status == StatusCancelled && status != StatusFailed {
		status = StatusCancelled
	}
	task.Status = status
	task.Result = resultText
	task.Error = errText
	task.EndedAt = time.Now()
	task.Usage = result.Usage
	task.ToolCount = result.ToolCount
	if result.LastTool != "" {
		task.LastActivity = "last tool: " + result.LastTool
	} else {
		task.LastActivity = string(status)
	}
	m.mu.Unlock()
	m.publishDone(id)
}

func (m *Manager) publishDone(id string) {
	select {
	case m.done <- DoneEvent{TaskID: id}:
	default:
	}
}

func statusFromStop(reason agent.StopReason) Status {
	switch reason {
	case agent.StopCompleted:
		return StatusCompleted
	case agent.StopMaxIterations:
		return StatusMaxTurns
	case agent.StopCanceled:
		return StatusCancelled
	default:
		return StatusFailed
	}
}

func defaultRun(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
	return runner.RunToCompletion(ctx, agent.RunToCompletionInput{
		Request:  agent.Request{Conversation: conv},
		TaskText: prompt,
	})
}

func defaultIDSource() IDSource {
	var n atomic.Uint64
	return func() string {
		return fmt.Sprintf("task-%d", n.Add(1))
	}
}
