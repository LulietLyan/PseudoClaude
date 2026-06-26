package team

import (
	"context"
	"fmt"
	"strings"
)

const mailboxWakePrompt = "You have new unread team mailbox messages. Read the incoming team messages in the system reminder, handle the request, and use SendMessage to report results or ask follow-up questions."

func (m *Manager) ResumeMember(ctx context.Context, teamName, recipient string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("team manager is nil")
	}
	if m.tasks == nil {
		return "", ErrBackendDisabled
	}
	team, ok := m.getInternal(teamName)
	if !ok {
		return "", ErrTeamNotFound
	}
	if err := team.reloadMembers(m.homeDir); err != nil {
		return "", err
	}
	member, ok := findMember(team.Members, recipient)
	if !ok {
		return "", ErrMemberNotFound
	}
	if member.BackendType != BackendInProcess {
		return "", ErrBackendDisabled
	}
	if member.IsActive != nil && *member.IsActive {
		return "", fmt.Errorf("member %q is still active", member.Name)
	}
	if err := team.SetMemberActive(m.homeDir, member.AgentID, true); err != nil {
		return "", err
	}
	taskID, err := m.resumeTaskByMember(ctx, member)
	if err != nil {
		active := strings.Contains(err.Error(), "still running")
		_ = team.SetMemberActive(m.homeDir, member.AgentID, active)
		return "", err
	}
	return taskID, nil
}

func (m *Manager) resumeTaskByMember(ctx context.Context, member MemberInfo) (string, error) {
	taskID, err := m.tasks.SendMessage(ctx, member.AgentID, mailboxWakePrompt)
	if err == nil {
		return taskID, nil
	}
	if member.Name != "" && member.Name != member.AgentID && strings.Contains(err.Error(), "not found") {
		return m.tasks.SendMessage(ctx, member.Name, mailboxWakePrompt)
	}
	return "", err
}
