// Package worktree manages isolated Git worktrees for sub Agent runs.
package worktree

import (
	"errors"
	"time"
)

const (
	DefaultMetaDirName     = ".PseudoClaude"
	DefaultWorktreeDirName = "worktrees"
	DefaultSessionFileName = "worktree_session.json"
	DefaultNameLimit       = 64
)

var DefaultSymlinkDirs = []string{"node_modules", ".venv", "vendor"}

var (
	ErrUnavailable      = errors.New("worktree manager unavailable")
	ErrInvalidName      = errors.New("invalid worktree name")
	ErrNotFound         = errors.New("worktree not found")
	ErrActiveMismatch   = errors.New("worktree is not the active session")
	ErrProtectedChanges = errors.New("worktree has protected changes")
)

type Worktree struct {
	Name       string
	FlatName   string
	Path       string
	Branch     string
	BasedOn    string
	HeadCommit string
	CreatedAt  time.Time
	Manual     bool
}

type Session struct {
	OriginalCWD        string    `json:"original_cwd"`
	WorktreePath       string    `json:"worktree_path"`
	WorktreeName       string    `json:"worktree_name"`
	OriginalBranch     string    `json:"original_branch,omitempty"`
	OriginalHeadCommit string    `json:"original_head_commit,omitempty"`
	SessionID          string    `json:"session_id"`
	StartedAt          time.Time `json:"started_at"`
}

type Options struct {
	RepoRoot    string
	MetaDirName string
	SymlinkDirs []string
	Logf        func(format string, args ...any)
	Git         GitRunner
	IDSource    func() string
	Now         func() time.Time
}

type CreateInput struct {
	Name    string
	BaseRef string
	Manual  bool
}

type ExitOptions struct {
	Remove  bool
	Discard bool
}

type RemoveOptions struct {
	Discard bool
}

type ExitReport struct {
	Name    string
	Path    string
	Branch  string
	Removed bool
}

type RemoveReport struct {
	Name   string
	Path   string
	Branch string
}

type AutoCleanupReport struct {
	Name   string
	Path   string
	Branch string
	Kept   bool
	Reason string
}

type Summary struct {
	Name       string
	Path       string
	Branch     string
	Manual     bool
	Active     bool
	Dirty      bool
	DirtyError string
}

type SweepResult struct {
	Name    string
	Path    string
	Removed bool
	Reason  string
	Err     error
}
