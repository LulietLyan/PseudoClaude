package team

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"PseudoClaude/internal/task"
	"PseudoClaude/internal/team/registry"
)

type Manager struct {
	mu          sync.RWMutex
	homeDir     string
	projectRoot string
	root        string
	teams       map[string]*Team
	registry    *registry.AgentNameRegistry
	backend     BackendController
	worktrees   WorktreeController
	tasks       *task.Manager
	logf        func(format string, args ...any)
}

func NewManager(opts ManagerOptions) (*Manager, error) {
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	m := &Manager{
		homeDir:     home,
		projectRoot: opts.ProjectRoot,
		root:        teamsRoot(home),
		teams:       map[string]*Team{},
		registry:    registry.New(),
		backend:     opts.Backend,
		worktrees:   opts.Worktrees,
		tasks:       opts.Tasks,
		logf:        opts.Logf,
	}
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return nil, err
	}
	if err := m.recoverTeams(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Create(ctx context.Context, in CreateInput) (*Team, error) {
	if m == nil {
		return nil, fmt.Errorf("team manager is nil")
	}
	base := SanitizeName(in.Name)
	safe, err := uniqueSanitizedName(m.root, base)
	if err != nil {
		return nil, err
	}
	backend := in.Backend
	if backend == "" {
		backend = BackendInProcess
	}
	team := &Team{
		Name:          in.Name,
		SanitizedName: safe,
		Description:   in.Description,
		ProjectRoot:   m.projectRoot,
		LeadAgentID:   in.LeadAgentID,
		Backend:       backend,
		CreatedAt:     time.Now(),
	}
	derivePaths(team, m.homeDir)
	if err := os.MkdirAll(team.InboxDir, 0o755); err != nil {
		return nil, err
	}
	if err := atomicWriteJSON(team.TasksPath, map[string]any{"tasks": []any{}}); err != nil {
		return nil, err
	}
	if team.LeadAgentID != "" {
		team.Members = append(team.Members, MemberInfo{
			Name:          "lead",
			AgentID:       team.LeadAgentID,
			BackendType:   backend,
			LastUpdatedAt: time.Now(),
		})
	}
	if err := team.save(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.teams[team.SanitizedName] = team
	if team.Name != team.SanitizedName {
		m.teams[team.Name] = team
	}
	for _, member := range team.Members {
		m.registry.Register(member.Name, member.AgentID)
	}
	m.mu.Unlock()
	return cloneTeam(team), nil
}

func (m *Manager) Get(name string) (*Team, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	team, ok := m.teams[name]
	if !ok {
		for _, candidate := range m.uniqueTeamsLocked() {
			if candidate.Name == name || candidate.SanitizedName == name {
				team, ok = candidate, true
				break
			}
		}
	}
	if !ok {
		return nil, false
	}
	return cloneTeam(team), true
}

func (m *Manager) List() []*Team {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	teams := m.uniqueTeamsLocked()
	sort.Slice(teams, func(i, j int) bool {
		return teams[i].CreatedAt.Before(teams[j].CreatedAt)
	})
	out := make([]*Team, 0, len(teams))
	for _, team := range teams {
		out = append(out, cloneTeam(team))
	}
	return out
}

func (m *Manager) HomeDir() string {
	if m == nil {
		return ""
	}
	return m.homeDir
}

func (m *Manager) Delete(ctx context.Context, name string, force bool) error {
	if m == nil {
		return fmt.Errorf("team manager is nil")
	}
	team, ok := m.getInternal(name)
	if !ok {
		return ErrTeamNotFound
	}
	if err := team.reloadMembers(m.homeDir); err != nil {
		return err
	}
	if !force {
		for _, member := range team.Members {
			if member.Name == "lead" {
				continue
			}
			if member.IsActive == nil || *member.IsActive {
				return ErrTeamActive
			}
		}
	}
	if force {
		for _, member := range team.Members {
			if member.Name != "lead" && m.backend != nil {
				if err := m.backend.Kill(ctx, KillRequest{TeamName: team.Name, MemberName: member.Name, AgentID: member.AgentID, PaneID: member.PaneID, Backend: member.BackendType}); err != nil {
					m.warn("failed to kill member %s/%s: %v", team.Name, member.Name, err)
				}
			}
			if member.SessionDir != "" {
				_ = os.RemoveAll(member.SessionDir)
			}
			if member.WorktreeName != "" && m.worktrees != nil {
				if _, err := m.worktrees.AutoCleanup(ctx, member.WorktreeName); err != nil {
					m.warn("failed to remove worktree %s: %v", member.WorktreeName, err)
				}
			} else if member.WorktreePath != "" {
				_ = os.RemoveAll(member.WorktreePath)
			}
		}
	}
	if err := os.RemoveAll(team.ConfigDir); err != nil {
		return err
	}
	m.mu.Lock()
	for key, candidate := range m.teams {
		if candidate.SanitizedName == team.SanitizedName {
			delete(m.teams, key)
		}
	}
	for _, member := range team.Members {
		m.registry.UnregisterByAgentID(member.AgentID)
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) KillMember(ctx context.Context, teamName, memberName string) error {
	team, ok := m.getInternal(teamName)
	if !ok {
		return ErrTeamNotFound
	}
	if err := team.reloadMembers(m.homeDir); err != nil {
		return err
	}
	member, ok := findMember(team.Members, memberName)
	if !ok {
		return ErrMemberNotFound
	}
	if m.backend != nil {
		if err := m.backend.Kill(ctx, KillRequest{TeamName: team.Name, MemberName: member.Name, AgentID: member.AgentID, PaneID: member.PaneID, Backend: member.BackendType}); err != nil {
			return err
		}
	}
	return team.RemoveMember(m.homeDir, memberName)
}

func (m *Manager) recoverTeams() error {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		team, err := loadTeam(filepath.Join(m.root, entry.Name(), configName), m.homeDir)
		if err != nil {
			m.warn("skipping damaged team config %s: %v", entry.Name(), err)
			continue
		}
		m.teams[team.SanitizedName] = team
		if team.Name != team.SanitizedName {
			m.teams[team.Name] = team
		}
		for _, member := range team.Members {
			m.registry.Register(member.Name, member.AgentID)
		}
	}
	return nil
}

func (m *Manager) getInternal(name string) (*Team, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	team, ok := m.teams[name]
	if ok {
		return team, true
	}
	for _, candidate := range m.uniqueTeamsLocked() {
		if candidate.Name == name || candidate.SanitizedName == name {
			return candidate, true
		}
	}
	return nil, false
}

func (m *Manager) uniqueTeamsLocked() []*Team {
	seen := map[string]bool{}
	out := []*Team{}
	for _, team := range m.teams {
		if seen[team.SanitizedName] {
			continue
		}
		seen[team.SanitizedName] = true
		out = append(out, team)
	}
	return out
}

func (m *Manager) warn(format string, args ...any) {
	if m.logf != nil {
		m.logf(format, args...)
	}
}

func cloneTeam(t *Team) *Team {
	if t == nil {
		return nil
	}
	cp := *t
	cp.Members = append([]MemberInfo(nil), t.Members...)
	return &cp
}
