package subagent

import (
	"strings"
	"testing"
)

func TestSourcePriority(t *testing.T) {
	if !(SourceProject.Priority() > SourceUser.Priority() &&
		SourceUser.Priority() > SourceBuiltin.Priority() &&
		SourceBuiltin.Priority() > SourcePlugin.Priority()) {
		t.Fatalf("source priorities out of order")
	}
}

func TestParseModelRef(t *testing.T) {
	model, warning := ParseModelRef("haiku")
	if model != ModelHaiku || !warning.Empty() {
		t.Fatalf("ParseModelRef(haiku) = %q, %#v", model, warning)
	}
	model, warning = ParseModelRef("mystery")
	if model != ModelInherit || warning.Empty() || warning.Field != "model" {
		t.Fatalf("invalid model did not fall back with warning: %q %#v", model, warning)
	}
}

func TestParsePermissionRef(t *testing.T) {
	perm, warning := ParsePermissionRef("dontAsk")
	if perm != PermissionDontAsk || !warning.Empty() {
		t.Fatalf("ParsePermissionRef(dontAsk) = %q, %#v", perm, warning)
	}
	perm, warning = ParsePermissionRef("wild")
	if perm != PermissionInherit || warning.Empty() || warning.Field != "permissionMode" {
		t.Fatalf("invalid permission did not fall back with warning: %q %#v", perm, warning)
	}
}

func TestWarningFormatting(t *testing.T) {
	got := FormatWarning(Warning{Path: "/tmp/a.md", Agent: "demo", Field: "model", Message: "bad"})
	for _, want := range []string{"/tmp/a.md", "agent=demo", "field=model", "bad"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatWarning missing %q in %q", want, got)
		}
	}
}
