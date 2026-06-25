package team

import (
	"fmt"
	"strings"
)

func BuildMemberPrompt(team Team, member MemberInfo) string {
	names := make([]string, 0, len(team.Members))
	for _, item := range team.Members {
		if item.Name != "" {
			names = append(names, item.Name)
		}
	}
	return strings.TrimSpace(fmt.Sprintf(`You are a PseudoClaude team member.

Team: %s
Member name: %s
Agent ID: %s
Lead agent ID: %s
Worktree: %s
Current members: %s

Collaboration rules:
- Work only inside your assigned worktree.
- Read incoming mailbox reminders before acting.
- Plain assistant replies are local to your own run; they are not sent to the Lead or teammates.
- Use SendMessage to report progress, ask questions, coordinate with teammates, or notify the Lead.
- Use TaskCreate, TaskUpdate, TaskList, and TaskGet for shared task state.
- Do not start additional team members from inside this member run.`,
		team.Name,
		member.Name,
		member.AgentID,
		team.LeadAgentID,
		member.WorktreePath,
		strings.Join(names, ", "),
	))
}
