package agent

import (
	"context"
	"fmt"
	"strings"
)

type TeamService interface {
	SpawnMember(ctx context.Context, in TeamLaunchInput) (TeamLaunchResult, error)
}

type TeamLaunchInput struct {
	TeamName         string
	MemberName       string
	Prompt           string
	Description      string
	SubagentType     string
	Model            string
	Fork             bool
	PlanModeRequired bool
	Parent           RunnerSnapshot
}

type TeamLaunchResult struct {
	TeamName     string `json:"team_name"`
	MemberName   string `json:"member_name"`
	AgentID      string `json:"agent_id"`
	Backend      string `json:"backend"`
	PaneID       string `json:"pane_id,omitempty"`
	WorktreePath string `json:"worktree_path"`
	SessionDir   string `json:"session_dir"`
}

type TeamRunContext struct {
	TeamName   string
	MemberName string
	AgentID    string
	LeadID     string
	Inbox      InboxReader
}

type InboxReader interface {
	ReadUnread(agentID string) ([]IndexedMessage, error)
	MarkRead(agentID string, indices []int) error
}

type IndexedMessage struct {
	Index   int
	Message TeamMessage
}

type TeamMessage struct {
	From    string
	To      string
	Type    string
	Summary string
	Content string
}

func (c *TeamRunContext) Reminder() string {
	if c == nil || c.Inbox == nil || c.AgentID == "" {
		return ""
	}
	messages, err := c.Inbox.ReadUnread(c.AgentID)
	if err != nil {
		return fmt.Sprintf("<incoming-messages>\nFailed to read team mailbox: %v\n</incoming-messages>", err)
	}
	if len(messages) == 0 {
		return ""
	}
	indices := make([]int, 0, len(messages))
	var b strings.Builder
	b.WriteString("<incoming-messages>\n")
	b.WriteString("Unread team messages. Plain assistant replies are not sent to teammates; use SendMessage for reports or collaboration.\n")
	for _, indexed := range messages {
		indices = append(indices, indexed.Index)
		msg := indexed.Message
		b.WriteString(fmt.Sprintf("\n- from=%s to=%s type=%s summary=%s", msg.From, msg.To, msg.Type, msg.Summary))
		if strings.TrimSpace(msg.Content) != "" {
			b.WriteString("\n  ")
			b.WriteString(strings.TrimSpace(msg.Content))
		}
	}
	b.WriteString("\n</incoming-messages>")
	if err := c.Inbox.MarkRead(c.AgentID, indices); err != nil {
		b.WriteString(fmt.Sprintf("\n<system-reminder>Failed to mark team messages read: %v</system-reminder>", err))
	}
	return b.String()
}
