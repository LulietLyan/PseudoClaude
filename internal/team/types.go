package team

import (
	"context"
	"errors"
	"time"

	"PseudoClaude/internal/task"
	"PseudoClaude/internal/worktree"
)

type BackendType string

const (
	BackendTmux      BackendType = "tmux"
	BackendIterm2    BackendType = "iterm2"
	BackendInProcess BackendType = "in-process"
)

type Team struct {
	Name          string       `json:"name"`
	SanitizedName string       `json:"sanitized_name"`
	Description   string       `json:"description,omitempty"`
	ProjectRoot   string       `json:"project_root"`
	LeadAgentID   string       `json:"lead_agent_id"`
	Backend       BackendType  `json:"backend"`
	CreatedAt     time.Time    `json:"created_at"`
	Members       []MemberInfo `json:"members"`

	ConfigDir  string `json:"-"`
	ConfigPath string `json:"-"`
	InboxDir   string `json:"-"`
	TasksPath  string `json:"-"`
}

type MemberInfo struct {
	Name             string      `json:"name"`
	AgentID          string      `json:"agent_id"`
	AgentType        string      `json:"agent_type,omitempty"`
	Model            string      `json:"model,omitempty"`
	WorktreeName     string      `json:"worktree_name"`
	WorktreePath     string      `json:"worktree_path"`
	Branch           string      `json:"branch"`
	BackendType      BackendType `json:"backend_type"`
	PaneID           string      `json:"pane_id,omitempty"`
	IsActive         *bool       `json:"is_active,omitempty"`
	PlanModeRequired bool        `json:"plan_mode_required"`
	SessionID        string      `json:"session_id"`
	SessionDir       string      `json:"session_dir"`
	LastUpdatedAt    time.Time   `json:"last_updated_at"`
}

type CreateInput struct {
	Name        string
	Description string
	LeadAgentID string
	Backend     BackendType
}

type ManagerOptions struct {
	HomeDir     string
	ProjectRoot string
	Logf        func(format string, args ...any)
	Backend     BackendController
	Worktrees   WorktreeController
	Tasks       *task.Manager
}

type LeadMessage struct {
	TeamName string
	Message  any
}

type BackendController interface {
	Kill(ctx context.Context, req KillRequest) error
}

type WorktreeController interface {
	Create(ctx context.Context, in worktree.CreateInput) (*worktree.Worktree, error)
	AutoCleanup(ctx context.Context, name string) (*worktree.AutoCleanupReport, error)
}

type KillRequest struct {
	TeamName   string
	MemberName string
	AgentID    string
	PaneID     string
	Backend    BackendType
}

var (
	ErrTeamNotFound     = errors.New("team not found")
	ErrTeamActive       = errors.New("team has active members")
	ErrMemberNotFound   = errors.New("member not found")
	ErrMemberExists     = errors.New("member already exists")
	ErrWorktreeDisabled = errors.New("worktree unavailable")
	ErrBackendDisabled  = errors.New("backend unavailable")
)
