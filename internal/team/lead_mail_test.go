package team

import (
	"context"
	"strings"
	"testing"

	"PseudoClaude/internal/team/mailbox"
)

func TestLeadReminderReadsAndMarksLeadMailbox(t *testing.T) {
	mgr, err := NewManager(ManagerOptions{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	team, err := mgr.Create(context.Background(), CreateInput{Name: "Demo", LeadAgentID: "lead-demo"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := mailbox.New(team.InboxDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write(context.Background(), "lead-demo", mailbox.Message{From: "agent-a", To: "lead-demo", Summary: "done", Content: "read result"}); err != nil {
		t.Fatal(err)
	}
	reminders := mgr.LeadReminder()
	if len(reminders) != 1 || !strings.Contains(reminders[0], "<team-update>") || !strings.Contains(reminders[0], "read result") {
		t.Fatalf("reminders = %#v", reminders)
	}
	unread, err := store.ReadUnread("lead-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 0 {
		t.Fatalf("unread after reminder = %#v", unread)
	}
}

func TestHasLeadMailPeeksWithoutMarkingRead(t *testing.T) {
	mgr, err := NewManager(ManagerOptions{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	team, err := mgr.Create(context.Background(), CreateInput{Name: "Demo", LeadAgentID: "lead-demo"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := mailbox.New(team.InboxDir)
	if err != nil {
		t.Fatal(err)
	}
	if mgr.HasLeadMail() {
		t.Fatal("unexpected lead mail")
	}
	if err := store.Write(context.Background(), "lead-demo", mailbox.Message{From: "agent-a", To: "lead-demo", Summary: "done"}); err != nil {
		t.Fatal(err)
	}
	if !mgr.HasLeadMail() {
		t.Fatal("expected lead mail")
	}
	unread, err := store.ReadUnread("lead-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 {
		t.Fatalf("HasLeadMail should not mark read: %#v", unread)
	}
}
