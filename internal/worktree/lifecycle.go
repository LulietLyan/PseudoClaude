package worktree

import (
	"context"
	"fmt"
)

func (m *Manager) Enter(ctx context.Context, name string) (*Session, error) {
	if m == nil {
		return nil, ErrUnavailable
	}
	m.mu.Lock()
	wt := m.active[name]
	m.mu.Unlock()
	if wt == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	branch, _ := runGitTrimmed(ctx, m.git, m.repoRoot, "branch", "--show-current")
	head, _ := runGitTrimmed(ctx, m.git, m.repoRoot, "rev-parse", "HEAD")
	sessionID := ""
	if m.idSource != nil {
		sessionID = m.idSource()
	}
	if sessionID == "" {
		sessionID = RandomAgentName()
	}
	s := &Session{
		OriginalCWD:        m.repoRoot,
		WorktreePath:       wt.Path,
		WorktreeName:       wt.Name,
		OriginalBranch:     branch,
		OriginalHeadCommit: head,
		SessionID:          sessionID,
		StartedAt:          m.now(),
	}
	if err := saveSession(m.sessionFile, s); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.current = s
	m.mu.Unlock()
	return s, nil
}

func (m *Manager) Exit(ctx context.Context, opts ExitOptions) (*ExitReport, error) {
	if m == nil {
		return nil, ErrUnavailable
	}
	m.mu.Lock()
	current := m.current
	m.mu.Unlock()
	if current == nil {
		return nil, ErrActiveMismatch
	}
	if !opts.Remove {
		if err := clearSession(m.sessionFile); err != nil {
			return nil, err
		}
		m.mu.Lock()
		m.current = nil
		m.mu.Unlock()
		return &ExitReport{Name: current.WorktreeName, Path: current.WorktreePath}, nil
	}
	report, err := m.Remove(ctx, current.WorktreeName, RemoveOptions{Discard: opts.Discard})
	if err != nil {
		return nil, err
	}
	return &ExitReport{Name: report.Name, Path: report.Path, Branch: report.Branch, Removed: true}, nil
}

func (m *Manager) Remove(ctx context.Context, name string, opts RemoveOptions) (*RemoveReport, error) {
	if m == nil {
		return nil, ErrUnavailable
	}
	m.mu.Lock()
	wt := m.active[name]
	m.mu.Unlock()
	if wt == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if !opts.Discard {
		if protected, reason := hasProtectedChanges(ctx, m.git, wt); protected {
			return nil, fmt.Errorf("%w: %s (%s, %s)", ErrProtectedChanges, reason, wt.Path, wt.Branch)
		}
	}
	if _, err := runGitTrimmed(ctx, m.git, m.repoRoot, "worktree", "remove", "--force", wt.Path); err != nil {
		return nil, err
	}
	_, _ = runGitTrimmed(ctx, m.git, m.repoRoot, "branch", "-D", wt.Branch)
	m.mu.Lock()
	delete(m.active, name)
	if m.current != nil && m.current.WorktreeName == name {
		m.current = nil
		_ = clearSession(m.sessionFile)
	}
	m.mu.Unlock()
	return &RemoveReport{Name: wt.Name, Path: wt.Path, Branch: wt.Branch}, nil
}

func (m *Manager) AutoCleanup(ctx context.Context, name string) (*AutoCleanupReport, error) {
	if m == nil {
		return nil, ErrUnavailable
	}
	m.mu.Lock()
	wt := m.active[name]
	m.mu.Unlock()
	if wt == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	report := &AutoCleanupReport{Name: wt.Name, Path: wt.Path, Branch: wt.Branch}
	if wt.Manual {
		report.Kept = true
		report.Reason = "manual worktree"
		return report, nil
	}
	if protected, reason := hasProtectedChanges(ctx, m.git, wt); protected {
		report.Kept = true
		report.Reason = reason
		return report, nil
	}
	if _, err := m.Remove(ctx, name, RemoveOptions{Discard: true}); err != nil {
		report.Kept = true
		report.Reason = err.Error()
		return report, nil
	}
	return report, nil
}
