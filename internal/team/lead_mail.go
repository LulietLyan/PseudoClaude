package team

import (
	"context"
	"fmt"
	"strings"

	"PseudoClaude/internal/team/mailbox"
)

func (m *Manager) LeadReminder() []string {
	if m == nil {
		return nil
	}
	var reminders []string
	for _, tm := range m.List() {
		if tm.LeadAgentID == "" {
			continue
		}
		store, err := mailbox.New(tm.InboxDir)
		if err != nil {
			m.warn("failed to open lead mailbox for %s: %v", tm.Name, err)
			continue
		}
		messages, err := store.ReadUnread(tm.LeadAgentID)
		if err != nil {
			m.warn("failed to read lead mailbox for %s: %v", tm.Name, err)
			continue
		}
		if len(messages) == 0 {
			continue
		}
		indices := make([]int, 0, len(messages))
		var b strings.Builder
		b.WriteString("<team-update>\n")
		b.WriteString(fmt.Sprintf("Team %s has unread member updates.\n", tm.Name))
		for _, indexed := range messages {
			indices = append(indices, indexed.Index)
			msg := indexed.Message
			b.WriteString(fmt.Sprintf("\n- from=%s type=%s summary=%s", msg.From, msg.Type, msg.Summary))
			if strings.TrimSpace(msg.Content) != "" {
				b.WriteString("\n  ")
				b.WriteString(strings.TrimSpace(msg.Content))
			}
		}
		b.WriteString("\n</team-update>")
		if err := store.MarkRead(context.Background(), tm.LeadAgentID, indices); err != nil {
			b.WriteString(fmt.Sprintf("\n<system-reminder>Failed to mark team updates read: %v</system-reminder>", err))
		}
		reminders = append(reminders, b.String())
	}
	return reminders
}

func (m *Manager) HasLeadMail() bool {
	if m == nil {
		return false
	}
	for _, tm := range m.List() {
		if tm.LeadAgentID == "" {
			continue
		}
		store, err := mailbox.New(tm.InboxDir)
		if err != nil {
			m.warn("failed to open lead mailbox for %s: %v", tm.Name, err)
			continue
		}
		messages, err := store.ReadUnread(tm.LeadAgentID)
		if err != nil {
			m.warn("failed to read lead mailbox for %s: %v", tm.Name, err)
			continue
		}
		if len(messages) > 0 {
			return true
		}
	}
	return false
}
