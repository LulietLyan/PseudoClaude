package team

import (
	"context"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/team/mailbox"
)

type AgentInbox struct {
	Store *mailbox.Store
}

func (i AgentInbox) ReadUnread(agentID string) ([]agent.IndexedMessage, error) {
	if i.Store == nil {
		return nil, nil
	}
	messages, err := i.Store.ReadUnread(agentID)
	if err != nil {
		return nil, err
	}
	out := make([]agent.IndexedMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, agent.IndexedMessage{
			Index: msg.Index,
			Message: agent.TeamMessage{
				From:    msg.Message.From,
				To:      msg.Message.To,
				Type:    string(msg.Message.Type),
				Summary: msg.Message.Summary,
				Content: msg.Message.Content,
			},
		})
	}
	return out, nil
}

func (i AgentInbox) MarkRead(agentID string, indices []int) error {
	if i.Store == nil {
		return nil
	}
	return i.Store.MarkRead(context.Background(), agentID, indices)
}
