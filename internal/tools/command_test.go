package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunCommand(t *testing.T) {
	dir := t.TempDir()
	env := DefaultEnv(dir)
	tool := NewRunCommandTool()

	result := tool.Execute(context.Background(), json.RawMessage(`{"command":"go","args":["env","GOVERSION"]}`), env)
	if !result.OK || !strings.Contains(result.Content, "go") || result.Metadata["exit_code"] != 0 {
		t.Fatalf("success result = %+v", result)
	}

	result = tool.Execute(context.Background(), json.RawMessage(`{"command":"go","args":["tool","definitely-not-a-tool"]}`), env)
	if result.OK || result.ErrorType != "command_failed" {
		t.Fatalf("failure result = %+v", result)
	}
	if _, ok := result.Metadata["exit_code"]; !ok {
		t.Fatalf("missing exit code: %+v", result)
	}

	env.MaxOutputBytes = 8
	result = tool.Execute(context.Background(), json.RawMessage(`{"command":"go","args":["env"]}`), env)
	if !result.OK || result.Metadata["stdout_truncated"] != true {
		t.Fatalf("truncate result = %+v", result)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	env := DefaultEnv(t.TempDir())
	env.Timeout = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), env.Timeout)
	defer cancel()
	result := NewRunCommandTool().Execute(ctx, json.RawMessage(`{"command":"sh","args":["-c","sleep 1"]}`), env)
	if result.OK || result.ErrorType != "timeout" {
		t.Fatalf("timeout result = %+v", result)
	}
}
