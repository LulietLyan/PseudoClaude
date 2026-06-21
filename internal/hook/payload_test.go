package hook

import (
	"strings"
	"testing"

	"PseudoClaude/internal/permission"
)

func TestPayload(t *testing.T) {
	payload := NewPayload(EventPreToolUse, "s1", "/work", permission.ModeDefault).
		With("tool_input", map[string]any{"path": "src/main.go", "count": 3, "ok": true}).
		With("z", "last").
		With("a", "first")
	data, err := payload.JSON()
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.HasPrefix(got, `{"a":"first","cwd":`) {
		t.Fatalf("json keys not stable: %s", got)
	}
	if gotPath := GetStringByPath(payload, "tool_input.path"); gotPath != "src/main.go" {
		t.Fatalf("path = %q", gotPath)
	}
	if got := GetStringByPath(payload, "tool_input.count"); got != "3" {
		t.Fatalf("number = %q", got)
	}
	if got := GetStringByPath(payload, "tool_input.ok"); got != "true" {
		t.Fatalf("bool = %q", got)
	}
	if got := GetStringByPath(payload, "missing.path"); got != "" {
		t.Fatalf("missing = %q", got)
	}
	if got := GetStringByPath(payload, "tool_input"); !strings.Contains(got, `"path":"src/main.go"`) {
		t.Fatalf("object = %q", got)
	}
}
