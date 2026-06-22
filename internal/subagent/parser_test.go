package subagent

import "testing"

func TestParseDefinitionComplete(t *testing.T) {
	data := []byte(`---
name: demo-agent
description: Demo role.
tools:
  - read_file
  - " "
  - search_code
  - read_file
disallowedTools:
  - write_file
model: opus
maxTurns: 5
permissionMode: acceptEdits
background: true
---
You are demo.
`)
	def, err := ParseDefinition("demo.md", SourceProject, data)
	if err != nil {
		t.Fatalf("ParseDefinition returned error: %v", err)
	}
	if def.Name != "demo-agent" || def.Description != "Demo role." || def.Model != ModelOpus || def.MaxTurns != 5 || def.Permission != PermissionAcceptEdits || !def.Background {
		t.Fatalf("definition fields not parsed: %#v", def)
	}
	if len(def.Tools) != 2 || def.Tools[0] != "read_file" || def.Tools[1] != "search_code" {
		t.Fatalf("tools not cleaned deterministically: %#v", def.Tools)
	}
	if len(def.DisallowedTools) != 1 || def.DisallowedTools[0] != "write_file" {
		t.Fatalf("disallowed tools not parsed: %#v", def.DisallowedTools)
	}
	if def.SystemPrompt != "You are demo." {
		t.Fatalf("system prompt = %q", def.SystemPrompt)
	}
}

func TestParseDefinitionErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"missing frontmatter", "name: nope"},
		{"invalid name", "---\nname: Bad_Name\ndescription: Bad.\n---\nBody"},
		{"missing description", "---\nname: demo\n---\nBody"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDefinition("bad.md", SourceUser, []byte(tt.data)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseDefinitionWarnings(t *testing.T) {
	def, err := ParseDefinition("warn.md", SourceUser, []byte(`---
name: warn
description: Warn role.
model: nope
permissionMode: yolo
maxTurns: -2
---
Body
`))
	if err != nil {
		t.Fatalf("ParseDefinition returned error: %v", err)
	}
	if def.Model != ModelInherit || def.Permission != PermissionInherit || def.MaxTurns != 0 {
		t.Fatalf("fallbacks not applied: %#v", def)
	}
	if len(def.Warnings) != 3 {
		t.Fatalf("warnings = %d, want 3: %#v", len(def.Warnings), def.Warnings)
	}
}
