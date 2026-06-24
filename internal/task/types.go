package task

import (
	"context"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusMaxTurns  Status = "max_turns"
)

type BackgroundTask struct {
	ID           string
	Name         string
	Type         string
	Fork         bool
	Status       Status
	Prompt       string
	Result       string
	Error        string
	StartedAt    time.Time
	EndedAt      time.Time
	Cancel       context.CancelFunc
	Runner       agent.Runner
	Conversation *conversation.Conversation
	Usage        llm.Usage
	ToolCount    int
	LastActivity string
}

type Snapshot struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	Type         string    `json:"type,omitempty"`
	Fork         bool      `json:"fork"`
	Status       Status    `json:"status"`
	Result       string    `json:"result,omitempty"`
	Error        string    `json:"error,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at,omitempty"`
	Usage        llm.Usage `json:"usage"`
	ToolCount    int       `json:"tool_count"`
	LastActivity string    `json:"last_activity,omitempty"`
}

type DoneEvent struct {
	TaskID string
}

type LaunchInput struct {
	Name         string
	Type         string
	Fork         bool
	Prompt       string
	Runner       agent.Runner
	Conversation *conversation.Conversation
	Prepare      agent.AgentPrepareFunc
}

type AdoptInput struct {
	LaunchInput
	Cancel context.CancelFunc
}

type IDSource func() string

type RunFunc func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult

type Options struct {
	IDSource    IDSource
	Run         RunFunc
	AutoTimeout time.Duration
}

func snapshotOf(t *BackgroundTask) Snapshot {
	if t == nil {
		return Snapshot{}
	}
	return Snapshot{
		ID:           t.ID,
		Name:         t.Name,
		Type:         t.Type,
		Fork:         t.Fork,
		Status:       t.Status,
		Result:       t.Result,
		Error:        t.Error,
		StartedAt:    t.StartedAt,
		EndedAt:      t.EndedAt,
		Usage:        t.Usage,
		ToolCount:    t.ToolCount,
		LastActivity: t.LastActivity,
	}
}
