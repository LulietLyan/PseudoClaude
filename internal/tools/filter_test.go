package tools

import (
	"context"
	"encoding/json"
	"testing"
)

type filterTool struct {
	name string
}

func (t filterTool) Definition() Definition {
	return Definition{Name: t.name, Description: t.name, Safety: SafetyReadOnly}
}

func (t filterTool) Execute(context.Context, json.RawMessage, Env) Result {
	return Success(t.name, "", nil)
}

func TestFilterSubAgentTools(t *testing.T) {
	reg, err := NewRegistry(
		filterTool{"Agent"},
		filterTool{"read_file"},
		filterTool{"write_file"},
		filterTool{"edit_file"},
		filterTool{"run_command"},
		filterTool{"custom"},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertNames(t, FilterSubAgentTools(reg, FilterPolicy{}), []string{"custom", "edit_file", "read_file", "run_command", "write_file"})
	assertNames(t, FilterSubAgentTools(reg, FilterPolicy{Fork: true}), []string{"Agent", "custom", "edit_file", "read_file", "run_command", "write_file"})
	assertNames(t, FilterSubAgentTools(reg, FilterPolicy{Background: true}), []string{"edit_file", "read_file", "run_command", "write_file"})
	assertNames(t, FilterSubAgentTools(reg, FilterPolicy{DefinitionDisallowed: []string{"write_file"}}), []string{"custom", "edit_file", "read_file", "run_command"})
	assertNames(t, FilterSubAgentTools(reg, FilterPolicy{DefinitionTools: []string{"read_file", "write_file", "missing"}}), []string{"read_file", "write_file"})
	assertNames(t, FilterSubAgentTools(reg, FilterPolicy{
		DefinitionTools:      []string{"read_file", "write_file"},
		DefinitionDisallowed: []string{"write_file"},
	}), []string{"read_file"})
}

func assertNames(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("names = %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("names = %#v, want %#v", got, want)
		}
	}
}
