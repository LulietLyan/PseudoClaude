package task

import (
	"strings"
	"testing"
)

func TestFormatNotification(t *testing.T) {
	got := FormatNotification(Snapshot{ID: "task-1", Name: "demo", Type: "explore", Status: StatusCompleted, Result: strings.Repeat("x", 2100)})
	for _, want := range []string{"<task-notification>", "id: task-1", "name: demo", "type: explore", "status: completed", "result: "} {
		if !strings.Contains(got, want) {
			t.Fatalf("notification missing %q in %q", want, got)
		}
	}
	if len(got) > 2300 {
		t.Fatalf("notification not truncated, len=%d", len(got))
	}
}
