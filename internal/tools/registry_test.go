package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type fakeTool struct {
	name     string
	safety   Safety
	executed *bool
	wait     bool
}

func (f fakeTool) Definition() Definition {
	return Definition{Name: f.name, Description: "fake", InputSchema: objectSchema(nil), Safety: f.safety}
}

func (f fakeTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	if f.executed != nil {
		*f.executed = true
	}
	if f.wait {
		<-ctx.Done()
		return Failure(f.name, "timeout", ctx.Err().Error(), nil)
	}
	return Success(f.name, "ok", nil)
}

func TestRegistryRegisterGetAndDefinitions(t *testing.T) {
	r, err := NewRegistry(fakeTool{name: "b"}, fakeTool{name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("a"); !ok {
		t.Fatal("expected registered tool")
	}
	defs := r.Definitions()
	if len(defs) != 2 || defs[0].Name != "a" || defs[1].Name != "b" {
		t.Fatalf("definitions not stable sorted: %+v", defs)
	}
}

func TestRegistryRejectsInvalidRegistrations(t *testing.T) {
	if _, err := NewRegistry(nil); err == nil {
		t.Fatal("expected nil tool error")
	}
	if _, err := NewRegistry(fakeTool{}); err == nil {
		t.Fatal("expected empty name error")
	}
	if _, err := NewRegistry(fakeTool{name: "x"}, fakeTool{name: "x"}); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestRegistryExecuteUnknownAndInvalidJSON(t *testing.T) {
	executed := false
	r, err := NewRegistry(fakeTool{name: "known", executed: &executed})
	if err != nil {
		t.Fatal(err)
	}
	result := r.Execute(context.Background(), Call{Name: "missing", Arguments: json.RawMessage(`{}`)}, DefaultEnv(t.TempDir()))
	if result.OK || result.ErrorType != "unknown_tool" {
		t.Fatalf("unknown result = %+v", result)
	}
	result = r.Execute(context.Background(), Call{Name: "known", Arguments: json.RawMessage(`{`)}, DefaultEnv(t.TempDir()))
	if result.OK || result.ErrorType != "invalid_arguments" {
		t.Fatalf("invalid json result = %+v", result)
	}
	if executed {
		t.Fatal("tool executed for invalid json")
	}
}

func TestRegistryExecuteTimeout(t *testing.T) {
	r, err := NewRegistry(fakeTool{name: "slow", wait: true})
	if err != nil {
		t.Fatal(err)
	}
	env := DefaultEnv(t.TempDir())
	env.Timeout = 10 * time.Millisecond
	result := r.Execute(context.Background(), Call{Name: "slow", Arguments: json.RawMessage(`{}`)}, env)
	if result.OK || result.ErrorType != "timeout" {
		t.Fatalf("timeout result = %+v", result)
	}
}

func TestDefaultRegistry(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"edit_file", "find_files", "read_file", "run_command", "search_code", "write_file"}
	defs := r.Definitions()
	if len(defs) != len(want) {
		t.Fatalf("definition count = %d, want %d", len(defs), len(want))
	}
	for i, name := range want {
		if defs[i].Name != name {
			t.Fatalf("defs[%d] = %q, want %q", i, defs[i].Name, name)
		}
		if defs[i].Description == "" || defs[i].InputSchema == nil {
			t.Fatalf("definition incomplete: %+v", defs[i])
		}
	}
}

func TestRegistryDefinitionsBySafetyAndQueries(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defs := r.DefinitionsBySafety(SafetyReadOnly)
	want := []string{"find_files", "read_file", "search_code"}
	if len(defs) != len(want) {
		t.Fatalf("readonly definition count = %d, want %d: %+v", len(defs), len(want), defs)
	}
	for i, name := range want {
		if defs[i].Name != name {
			t.Fatalf("defs[%d] = %q, want %q", i, defs[i].Name, name)
		}
		if defs[i].Safety != SafetyReadOnly {
			t.Fatalf("defs[%d].Safety = %q, want read only", i, defs[i].Safety)
		}
	}
	if !r.IsKnown("read_file") || r.IsKnown("missing") {
		t.Fatalf("known query mismatch")
	}
	if safety, ok := r.Safety("write_file"); !ok || safety != SafetySideEffect {
		t.Fatalf("write_file safety = %q, %v", safety, ok)
	}
	if _, ok := r.Safety("missing"); ok {
		t.Fatal("missing tool should not have safety")
	}
}

func TestResultJSON(t *testing.T) {
	result := Success("x", "hello", map[string]any{"n": 1})
	var decoded Result
	if err := json.Unmarshal([]byte(result.JSON()), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK || decoded.Tool != "x" || decoded.Content != "hello" {
		t.Fatalf("decoded = %+v", decoded)
	}
}
