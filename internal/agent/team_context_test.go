package agent

import (
	"strings"
	"testing"
)

func TestTeamRunContextReminderReadsAndMarksMessages(t *testing.T) {
	inbox := &fakeInboxReader{
		messages: []IndexedMessage{{
			Index: 2,
			Message: TeamMessage{
				From:    "lead",
				To:      "agent-a",
				Type:    "text",
				Summary: "next task",
				Content: "Please continue.",
			},
		}},
	}
	ctx := &TeamRunContext{TeamName: "demo", MemberName: "alice", AgentID: "agent-a", Inbox: inbox}
	reminder := ctx.Reminder()
	if !strings.Contains(reminder, "<incoming-messages>") || !strings.Contains(reminder, "next task") || !strings.Contains(reminder, "Please continue.") {
		t.Fatalf("reminder = %q", reminder)
	}
	if len(inbox.marked) != 1 || inbox.marked[0] != 2 {
		t.Fatalf("marked = %#v", inbox.marked)
	}
}

type fakeInboxReader struct {
	messages []IndexedMessage
	marked   []int
}

func (f *fakeInboxReader) ReadUnread(agentID string) ([]IndexedMessage, error) {
	return f.messages, nil
}

func (f *fakeInboxReader) MarkRead(agentID string, indices []int) error {
	f.marked = append(f.marked, indices...)
	return nil
}
