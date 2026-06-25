package agent

import (
	"context"
	"sync"

	"PseudoClaude/internal/compact"
	"PseudoClaude/internal/config"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/hook"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/memory"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/prompt"
	"PseudoClaude/internal/session"
	"PseudoClaude/internal/tools"
)

type ApprovalUpgrader func(ctx context.Context, req ApprovalRequest) (permission.ApprovalDecision, error)

type SubRunOptions struct {
	SystemPrompt      string
	MaxTurns          int
	PermissionMode    permission.Mode
	DontAsk           bool
	IsSubAgent        bool
	IsFork            bool
	ParentLabel       string
	ApprovalUpgrader  ApprovalUpgrader
	FileCacheScope    string
	PendingReminderFn func() []string
}

type RunnerHandle struct {
	mu sync.RWMutex
	s  RunnerSnapshot
}

type RunnerSnapshot struct {
	Provider       llm.Provider
	Registry       *tools.Registry
	Env            tools.Env
	Config         Config
	Version        string
	Permission     *permission.Engine
	Compact        *compact.Runtime
	Instructions   string
	Memory         MemoryUpdater
	SkillsCatalog  func() []prompt.SkillCatalogItem
	ActiveSkills   func() []prompt.ActiveSkillEntry
	AllowedTools   []string
	Hooks          *hook.Engine
	HookPrompts    *hook.PromptQueue
	SessionID      string
	CWD            string
	Team           *TeamRunContext
	Conversation   *conversation.Conversation
	PermissionMode permission.Mode
	Providers      []config.ProviderConfig
	SessionContext session.Context
	MemoryManager  *memory.Manager
	Sub            SubRunOptions
	Approval       ApprovalUpgrader
}

func (h *RunnerHandle) Store(s RunnerSnapshot) {
	if h == nil {
		return
	}
	s.AllowedTools = append([]string(nil), s.AllowedTools...)
	h.mu.Lock()
	h.s = s
	h.mu.Unlock()
}

func (h *RunnerHandle) Snapshot() RunnerSnapshot {
	if h == nil {
		return RunnerSnapshot{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	s := h.s
	s.AllowedTools = append([]string(nil), s.AllowedTools...)
	return s
}

func (r Runner) WithSubOptions(sub SubRunOptions) Runner {
	r.Sub = sub
	r.AllowedTools = append([]string(nil), r.AllowedTools...)
	return r
}

func (s SubRunOptions) label() string {
	if s.ParentLabel != "" {
		return s.ParentLabel
	}
	if s.IsFork {
		return "fork"
	}
	if s.IsSubAgent {
		return "subagent"
	}
	return ""
}
