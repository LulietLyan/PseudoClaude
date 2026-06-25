package team

import (
	"fmt"

	"PseudoClaude/internal/team/mailbox"
	"PseudoClaude/internal/team/teamtask"
)

func (m *Manager) TaskStore(teamName string) (*teamtask.Store, error) {
	team, ok := m.getInternal(teamName)
	if !ok {
		return nil, ErrTeamNotFound
	}
	return teamtask.New(team.TasksPath), nil
}

func (m *Manager) MailboxStore(teamName string) (*mailbox.Store, error) {
	team, ok := m.getInternal(teamName)
	if !ok {
		return nil, ErrTeamNotFound
	}
	return mailbox.New(team.InboxDir)
}

func (m *Manager) ResolveMember(teamName, recipient string) (MemberInfo, error) {
	team, ok := m.getInternal(teamName)
	if !ok {
		return MemberInfo{}, ErrTeamNotFound
	}
	if err := team.reloadMembers(m.homeDir); err != nil {
		return MemberInfo{}, err
	}
	for _, member := range team.Members {
		if member.Name == recipient || member.AgentID == recipient {
			return member, nil
		}
	}
	return MemberInfo{}, fmt.Errorf("%w: %s", ErrMemberNotFound, recipient)
}

func (m *Manager) TeamMembers(teamName string) ([]MemberInfo, error) {
	team, ok := m.getInternal(teamName)
	if !ok {
		return nil, ErrTeamNotFound
	}
	if err := team.reloadMembers(m.homeDir); err != nil {
		return nil, err
	}
	return append([]MemberInfo(nil), team.Members...), nil
}
