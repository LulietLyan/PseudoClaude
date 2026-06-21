package hook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExecutorPromptOrSubagent(t *testing.T) {
	logs := []string{}
	exec := Executor{Logf: func(format string, args ...any) { logs = append(logs, formatString(format, args...)) }}
	prompt := Rule{Name: "p", Action: Action{Type: ActionPrompt, Prompt: &PromptAction{Text: "remember"}}}
	if got := exec.Run(context.Background(), prompt, Payload{}, false); got.Prompt != "remember" || got.Blocked || got.Err != nil {
		t.Fatalf("prompt result = %+v", got)
	}
	sub := Rule{Name: "s", Action: Action{Type: ActionSubagent, Subagent: &SubagentAction{AgentName: "worker", Prompt: "go"}}}
	if got := exec.Run(context.Background(), sub, Payload{}, false); got.Blocked || got.Err != nil {
		t.Fatalf("subagent result = %+v", got)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "not yet implemented") || !strings.Contains(logs[0], "worker") {
		t.Fatalf("logs = %+v", logs)
	}
}

func TestExecutorShell(t *testing.T) {
	exec := Executor{}
	payload := Payload{"event": "PreToolUse", "tool_name": "write_file"}
	block := Rule{Name: "block", Timeout: time.Second, Action: Action{Type: ActionShell, Shell: &ShellAction{Command: "cat >/tmp/hook-stdin; echo denied >&2; exit 2"}}}
	got := exec.Run(context.Background(), block, payload, true)
	if !got.Blocked || got.Reason != "denied" || got.Err != nil {
		t.Fatalf("block result = %+v", got)
	}
	pass := Rule{Name: "pass", Timeout: time.Second, Action: Action{Type: ActionShell, Shell: &ShellAction{Command: "exit 0"}}}
	if got := exec.Run(context.Background(), pass, payload, true); got.Blocked || got.Err != nil {
		t.Fatalf("pass result = %+v", got)
	}
	fail := Rule{Name: "fail", Timeout: time.Second, Action: Action{Type: ActionShell, Shell: &ShellAction{Command: "echo ordinary-fail >&2; exit 1"}}}
	if got := exec.Run(context.Background(), fail, payload, true); got.Blocked || got.Err == nil {
		t.Fatalf("ordinary failure should not block: %+v", got)
	}
	timeout := Rule{Name: "timeout", Timeout: 10 * time.Millisecond, Action: Action{Type: ActionShell, Shell: &ShellAction{Command: "sleep 1"}}}
	if got := exec.Run(context.Background(), timeout, payload, false); got.Blocked || got.Err == nil {
		t.Fatalf("timeout failure should not block: %+v", got)
	}
}

func TestExecutorHTTP(t *testing.T) {
	var seenMethod, seenHeader, seenBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenHeader = r.Header.Get("X-Hook")
		data := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(data)
		seenBody = string(data)
		if strings.Contains(seenBody, "block-me") {
			_, _ = w.Write([]byte(`{"decision":"block","reason":"network policy"}`))
			return
		}
		_, _ = w.Write([]byte(`{"decision":"allow"}`))
	}))
	defer server.Close()

	exec := Executor{Client: server.Client()}
	payload := Payload{"event": "Stop", "tool_name": "read_file"}
	rule := Rule{Name: "http", Timeout: time.Second, Action: Action{Type: ActionHTTP, HTTP: &HTTPAction{
		URL: server.URL, Headers: map[string]string{"X-Hook": "yes"},
	}}}
	if got := exec.Run(context.Background(), rule, payload, false); got.Err != nil || got.Blocked {
		t.Fatalf("http result = %+v", got)
	}
	if seenMethod != http.MethodPost || seenHeader != "yes" {
		t.Fatalf("request method/header = %s/%s", seenMethod, seenHeader)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(seenBody), &decoded); err != nil || decoded["event"] != "Stop" {
		t.Fatalf("default body = %q err=%v", seenBody, err)
	}

	block := Rule{Name: "block", Timeout: time.Second, Action: Action{Type: ActionHTTP, HTTP: &HTTPAction{
		URL: server.URL, Method: http.MethodPut, Body: `{"value":"{{.tool_name}} block-me"}`,
	}}}
	if got := exec.Run(context.Background(), block, payload, true); !got.Blocked || got.Reason != "network policy" || got.Err != nil {
		t.Fatalf("block result = %+v", got)
	}
	if seenMethod != http.MethodPut || !strings.Contains(seenBody, "read_file block-me") {
		t.Fatalf("templated request = method %s body %q", seenMethod, seenBody)
	}

	bad := Rule{Name: "bad", Timeout: time.Second, Action: Action{Type: ActionHTTP, HTTP: &HTTPAction{URL: "http://127.0.0.1:1"}}}
	if got := exec.Run(context.Background(), bad, payload, true); got.Blocked || got.Err == nil {
		t.Fatalf("network error should not block: %+v", got)
	}
}
