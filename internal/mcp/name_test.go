package mcp

import "testing"

func TestToolName(t *testing.T) {
	name := FullToolName("github", "get_issue")
	if name != "mcp__github__get_issue" {
		t.Fatalf("name = %q", name)
	}
	if !ValidToolName(name) {
		t.Fatalf("%q should be valid", name)
	}
	server, tool, ok := SplitToolName(name)
	if !ok || server != "github" || tool != "get_issue" {
		t.Fatalf("split = %q %q %v", server, tool, ok)
	}

	for _, invalid := range []string{"mcp__github.com__tool", "mcp__github__bad@tool", ""} {
		if ValidToolName(invalid) {
			t.Fatalf("%q should be invalid", invalid)
		}
	}
	for _, invalid := range []string{"read_file", "mcp____tool", "mcp__server__", "mcp__a__b__c"} {
		if _, _, ok := SplitToolName(invalid); ok {
			t.Fatalf("%q should not split", invalid)
		}
	}
}
