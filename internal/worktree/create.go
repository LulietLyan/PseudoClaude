package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (m *Manager) Create(ctx context.Context, in CreateInput) (*Worktree, error) {
	if m == nil {
		return nil, ErrUnavailable
	}
	flat, err := FlatName(in.Name)
	if err != nil {
		return nil, err
	}
	base := in.BaseRef
	if base == "" {
		base = "HEAD"
	}
	path := filepath.Join(m.worktreeDir, flat)
	branch := branchName(flat)
	m.mu.Lock()
	if existing := m.active[in.Name]; existing != nil {
		cp := *existing
		m.mu.Unlock()
		return &cp, nil
	}
	if _, ok := m.creating[in.Name]; ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("worktree %q is already being created", in.Name)
	}
	m.creating[in.Name] = struct{}{}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.creating, in.Name)
		m.mu.Unlock()
	}()
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		if wt, recoverErr := fastRecover(path); recoverErr == nil {
			wt.Name = in.Name
			wt.FlatName = flat
			wt.Path = path
			wt.BasedOn = base
			wt.CreatedAt = m.now()
			wt.Manual = in.Manual
			m.mu.Lock()
			m.active[in.Name] = wt
			m.mu.Unlock()
			return wt, nil
		}
	}
	if _, err := runGitTrimmed(ctx, m.git, m.repoRoot, "worktree", "add", "-B", branch, path, base); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("create worktree %q at %s: %w", in.Name, path, err)
	}
	head, err := runGitTrimmed(ctx, m.git, path, "rev-parse", "HEAD")
	if err != nil {
		_ = os.RemoveAll(path)
		return nil, err
	}
	wt := &Worktree{
		Name:       in.Name,
		FlatName:   flat,
		Path:       path,
		Branch:     branch,
		BasedOn:    base,
		HeadCommit: head,
		CreatedAt:  m.now(),
		Manual:     in.Manual,
	}
	m.postCreateSetup(ctx, wt)
	m.mu.Lock()
	m.active[in.Name] = wt
	m.mu.Unlock()
	return wt, nil
}
