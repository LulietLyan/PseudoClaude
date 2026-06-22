package command

import (
	"strings"
	"testing"
)

func TestFormatAgents(t *testing.T) {
	got := FormatAgents([]AgentSummary{
		{Name: "explore", Description: "Explore.", Source: "builtin", Model: "haiku", DisallowedTools: []string{"write_file"}},
		{Name: "plan", Description: "Plan.", Source: "builtin", Model: "sonnet"},
	})
	for _, want := range []string{"Loaded sub agents (2)", "explore", "plan", "source=builtin", "disallowed=write_file"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatAgents missing %q in %q", want, got)
		}
	}
}

func TestFormatAgentDetail(t *testing.T) {
	got := FormatAgentDetail(AgentDetail{
		Active:     AgentSummary{Name: "explore", Description: "Project explore.", Source: "project", Model: "haiku"},
		Overridden: []AgentSummary{{Name: "explore", Source: "builtin", Model: "haiku"}},
		Prompt:     "Read only.",
	})
	for _, want := range []string{"Sub agent: explore", "Project explore.", "Overridden sources", "builtin", "Read only."} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatAgentDetail missing %q in %q", want, got)
		}
	}
}
