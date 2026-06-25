package backend

import (
	"context"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/task"
	"PseudoClaude/internal/team"
)

type Backend interface {
	Type() team.BackendType
	Spawn(ctx context.Context, req SpawnRequest) (SpawnResult, error)
	Wake(ctx context.Context, req WakeRequest) error
	Kill(ctx context.Context, req KillRequest) error
}

type SpawnRequest struct {
	ExecutablePath   string
	ConfigPath       string
	ProjectRoot      string
	TeamName         string
	MemberName       string
	AgentID          string
	AgentType        string
	Model            string
	WorktreePath     string
	SessionID        string
	SessionDir       string
	PlanModeRequired bool
	InitialPrompt    string

	InProcessRunner  agent.Runner
	InProcessConv    *conversation.Conversation
	InProcessTaskMgr *task.Manager
}

type SpawnResult struct {
	AgentID string
	PaneID  string
}

type WakeRequest struct {
	TeamName   string
	MemberName string
	AgentID    string
	PaneID     string
}

type KillRequest struct {
	TeamName   string
	MemberName string
	AgentID    string
	PaneID     string
}

type Factory interface {
	Backend(typ team.BackendType) (Backend, error)
	Detect() (team.BackendType, error)
}

type FactoryDeps struct {
	Env      func(string) string
	LookPath func(string) (string, error)
}
