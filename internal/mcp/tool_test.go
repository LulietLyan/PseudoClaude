package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"PseudoClaude/internal/tools"
)

func TestAdaptTool(t *testing.T) {
	session := &stubSession{}
	tool, issue := AdaptTool("github", RemoteTool{Name: "get_issue", ReadOnly: true}, session, time.Second)
	if issue != nil {
		t.Fatalf("issue = %+v", issue)
	}
	def := tool.Definition()
	if def.Name != "mcp__github__get_issue" {
		t.Fatalf("name = %q", def.Name)
	}
	if def.Description == "" {
		t.Fatal("missing fallback description")
	}
	if def.InputSchema["type"] != "object" {
		t.Fatalf("schema = %+v", def.InputSchema)
	}
	if def.Safety != tools.SafetyReadOnly {
		t.Fatalf("safety = %s", def.Safety)
	}

	_, issue = AdaptTool("bad.server", RemoteTool{Name: "tool"}, session, time.Second)
	if issue == nil {
		t.Fatal("expected invalid name issue")
	}
	tool, issue = AdaptTool("github", RemoteTool{Name: "create_issue"}, session, time.Second)
	if issue != nil {
		t.Fatal(issue)
	}
	if got := tool.Definition().Safety; got != tools.SafetySideEffect {
		t.Fatalf("safety = %s", got)
	}
}

func TestRemoteToolExecute(t *testing.T) {
	session := &stubSession{
		callResult: CallResult{TextBlocks: []string{"one", "two"}, NonTextDropped: 1},
	}
	tool, issue := AdaptTool("github", RemoteTool{Name: "get_issue"}, session, time.Second)
	if issue != nil {
		t.Fatal(issue)
	}
	result := tool.Execute(context.Background(), json.RawMessage(`{"number":1}`), tools.Env{})
	if !result.OK || result.Content != "one\ntwo" {
		t.Fatalf("result = %+v", result)
	}
	if session.lastName != "get_issue" || session.lastArgs["number"].(float64) != 1 {
		t.Fatalf("call = %q %+v", session.lastName, session.lastArgs)
	}
	if result.Metadata["non_text_dropped"] != 1 {
		t.Fatalf("metadata = %+v", result.Metadata)
	}
}

func TestRemoteToolExecuteErrors(t *testing.T) {
	tool, issue := AdaptTool("github", RemoteTool{Name: "get_issue"}, &stubSession{}, time.Second)
	if issue != nil {
		t.Fatal(issue)
	}
	if got := tool.Execute(context.Background(), json.RawMessage(`[]`), tools.Env{}); got.OK || got.ErrorType != "invalid_arguments" {
		t.Fatalf("invalid args result = %+v", got)
	}

	remoteErrorSession := &stubSession{callResult: CallResult{IsError: true, TextBlocks: []string{"remote failed"}}}
	tool, _ = AdaptTool("github", RemoteTool{Name: "get_issue"}, remoteErrorSession, time.Second)
	if got := tool.Execute(context.Background(), json.RawMessage(`{}`), tools.Env{}); got.OK || got.ErrorType != "mcp_tool_error" || got.Error != "remote failed" {
		t.Fatalf("remote error result = %+v", got)
	}

	callErrorSession := &stubSession{callErr: errors.New("transport closed")}
	tool, _ = AdaptTool("github", RemoteTool{Name: "get_issue"}, callErrorSession, time.Second)
	if got := tool.Execute(context.Background(), json.RawMessage(`{}`), tools.Env{}); got.OK || got.ErrorType != "mcp_call_error" {
		t.Fatalf("call error result = %+v", got)
	}
}

func TestRemoteToolExecuteTimeout(t *testing.T) {
	session := &stubSession{block: true}
	tool, issue := AdaptTool("github", RemoteTool{Name: "slow"}, session, 10*time.Millisecond)
	if issue != nil {
		t.Fatal(issue)
	}
	got := tool.Execute(context.Background(), json.RawMessage(`{}`), tools.Env{})
	if got.OK || got.ErrorType != "mcp_call_error" {
		t.Fatalf("timeout result = %+v", got)
	}
}

type stubSession struct {
	listTools  []RemoteTool
	listErr    error
	callResult CallResult
	callErr    error
	closeErr   error
	block      bool
	closed     bool
	lastName   string
	lastArgs   map[string]any
}

func (s *stubSession) ListTools(ctx context.Context) ([]RemoteTool, error) {
	return s.listTools, s.listErr
}

func (s *stubSession) CallTool(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
	s.lastName = name
	s.lastArgs = arguments
	if s.block {
		<-ctx.Done()
		return CallResult{}, ctx.Err()
	}
	return s.callResult, s.callErr
}

func (s *stubSession) Close() error {
	s.closed = true
	return s.closeErr
}
