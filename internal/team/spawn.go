package team

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/task"
	"PseudoClaude/internal/team/mailbox"
	toolkit "PseudoClaude/internal/tools"
	"PseudoClaude/internal/worktree"
)

func (m *Manager) SpawnMember(ctx context.Context, in agent.TeamLaunchInput) (agent.TeamLaunchResult, error) {
	if m == nil {
		return agent.TeamLaunchResult{}, fmt.Errorf("team manager is nil")
	}
	team, ok := m.getInternal(in.TeamName)
	if !ok {
		return agent.TeamLaunchResult{}, ErrTeamNotFound
	}
	memberName := SanitizeName(in.MemberName)
	if memberName == "" {
		memberName = "member"
	}
	if m.worktrees == nil {
		return agent.TeamLaunchResult{}, ErrWorktreeDisabled
	}
	if team.Backend == BackendInProcess {
		if m.tasks == nil {
			return agent.TeamLaunchResult{}, ErrBackendDisabled
		}
		if in.Parent.Registry == nil || in.Parent.Provider == nil {
			return agent.TeamLaunchResult{}, ErrBackendDisabled
		}
	}
	agentID := "team-" + team.SanitizedName + "-" + memberName + "-" + fmt.Sprintf("%d", time.Now().UnixNano())
	worktreeName := "team-" + team.SanitizedName + "-" + memberName
	wt, err := m.worktrees.Create(ctx, worktree.CreateInput{Name: worktreeName})
	if err != nil {
		return agent.TeamLaunchResult{}, err
	}
	sessionID := agentID
	sessionDir := filepath.Join(m.homeDir, ".PseudoClaude", "teams", team.SanitizedName, "sessions", agentID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return agent.TeamLaunchResult{}, err
	}
	store, err := mailbox.New(team.InboxDir)
	if err != nil {
		return agent.TeamLaunchResult{}, err
	}
	if err := store.Write(ctx, agentID, mailbox.Message{
		From:    team.LeadAgentID,
		To:      agentID,
		Type:    mailbox.MessageText,
		Summary: in.Description,
		Content: in.Prompt,
	}); err != nil {
		return agent.TeamLaunchResult{}, err
	}
	member := MemberInfo{
		Name:             memberName,
		AgentID:          agentID,
		AgentType:        in.SubagentType,
		Model:            in.Model,
		WorktreeName:     wt.Name,
		WorktreePath:     wt.Path,
		Branch:           wt.Branch,
		BackendType:      team.Backend,
		IsActive:         boolPtr(true),
		PlanModeRequired: in.PlanModeRequired,
		SessionID:        sessionID,
		SessionDir:       sessionDir,
		LastUpdatedAt:    time.Now(),
	}
	if err := team.AddMember(m.homeDir, member); err != nil {
		return agent.TeamLaunchResult{}, err
	}
	m.registry.Register(member.Name, member.AgentID)
	if team.Backend == BackendInProcess {
		if err := m.launchInProcess(ctx, team, member, in, store); err != nil {
			_ = team.RemoveMember(m.homeDir, member.AgentID)
			m.registry.UnregisterByAgentID(member.AgentID)
			return agent.TeamLaunchResult{}, err
		}
	}
	return agent.TeamLaunchResult{
		TeamName:     team.Name,
		MemberName:   member.Name,
		AgentID:      member.AgentID,
		Backend:      string(member.BackendType),
		WorktreePath: member.WorktreePath,
		SessionDir:   member.SessionDir,
	}, nil
}

func (m *Manager) launchInProcess(ctx context.Context, team *Team, member MemberInfo, in agent.TeamLaunchInput, store *mailbox.Store) error {
	parent := in.Parent
	if parent.Registry == nil || parent.Provider == nil {
		return ErrBackendDisabled
	}
	runner := agent.Runner{
		Provider:      parent.Provider,
		Registry:      parent.Registry,
		Env:           parent.Env,
		Config:        parent.Config,
		Version:       parent.Version,
		Permission:    parent.Permission,
		Instructions:  parent.Instructions,
		AllowedTools:  toolkit.FilterSubAgentTools(parent.Registry, toolkit.FilterPolicy{TeamMember: true, InProcessTeamMember: true, Background: true}),
		Hooks:         parent.Hooks,
		HookPrompts:   parent.HookPrompts,
		SessionID:     member.SessionID,
		CWD:           member.WorktreePath,
		SkillsCatalog: parent.SkillsCatalog,
		ActiveSkills:  parent.ActiveSkills,
		Team: &agent.TeamRunContext{
			TeamName:   team.Name,
			MemberName: member.Name,
			AgentID:    member.AgentID,
			LeadID:     team.LeadAgentID,
			Inbox:      AgentInbox{Store: store},
		},
		Sub: agent.SubRunOptions{
			SystemPrompt:   BuildMemberPrompt(*team, member),
			IsSubAgent:     true,
			ParentLabel:    member.Name,
			DontAsk:        true,
			FileCacheScope: member.AgentID,
			PermissionMode: parent.PermissionMode,
		},
	}
	runner.Env.CWD = member.WorktreePath
	runner.Env.Team = &toolkit.TeamEnv{TeamName: team.Name, MemberName: member.Name, AgentID: member.AgentID}
	_, err := m.tasks.Launch(ctx, task.LaunchInput{
		ID:           member.AgentID,
		Name:         member.AgentID,
		Type:         member.AgentType,
		Prompt:       in.Prompt,
		Runner:       runner,
		Conversation: &conversation.Conversation{},
		OnFinish: func(ctx context.Context, event task.FinishEvent) {
			m.markMemberIdleAndNotify(ctx, team, member, event)
		},
	})
	return err
}

func (m *Manager) markMemberIdleAndNotify(ctx context.Context, team *Team, member MemberInfo, event task.FinishEvent) {
	if err := team.SetMemberActive(m.homeDir, member.AgentID, false); err != nil {
		m.warn("failed to mark member idle %s/%s: %v", team.Name, member.Name, err)
	}
	if team.LeadAgentID == "" {
		return
	}
	store, err := mailbox.New(team.InboxDir)
	if err != nil {
		m.warn("failed to open lead mailbox for %s: %v", team.Name, err)
		return
	}
	summary := "member idle"
	if event.Snapshot.Status != "" {
		summary = "member idle: " + string(event.Snapshot.Status)
	}
	if err := store.Write(ctx, team.LeadAgentID, mailbox.Message{
		From:    member.AgentID,
		To:      team.LeadAgentID,
		Type:    mailbox.MessageText,
		Summary: summary,
		Content: event.Snapshot.Result,
		Payload: map[string]any{
			"member_name": member.Name,
			"task_id":     event.TaskID,
			"status":      event.Snapshot.Status,
			"error":       event.Snapshot.Error,
		},
	}); err != nil {
		m.warn("failed to write lead idle notification for %s/%s: %v", team.Name, member.Name, err)
	}
}
