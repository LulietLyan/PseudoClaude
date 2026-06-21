package hook

import (
	"testing"

	"PseudoClaude/internal/permission"
)

func mustMatcher(t *testing.T, pattern string) permission.Matcher {
	t.Helper()
	m, err := permission.CompileMatcher(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCondition(t *testing.T) {
	payload := Payload{
		"tool_name": "write_file",
		"tool_input": map[string]any{
			"path": "src/main.go",
		},
	}
	if !EvalCondition(nil, payload) {
		t.Fatal("nil condition should match")
	}
	all := &Condition{Mode: CombineAllOf, Atoms: []Atom{
		{Field: "tool_name", Matcher: mustMatcher(t, "=write_file")},
		{Field: "tool_input.path", Matcher: mustMatcher(t, "**/*.go")},
	}}
	if !EvalCondition(all, payload) {
		t.Fatal("all_of should match")
	}
	any := &Condition{Mode: CombineAnyOf, Atoms: []Atom{
		{Field: "missing", Matcher: mustMatcher(t, "=x")},
		{Field: "tool_name", Matcher: mustMatcher(t, "~^write")},
	}}
	if !EvalCondition(any, payload) {
		t.Fatal("any_of should match")
	}
	not := &Condition{Mode: CombineAllOf, Atoms: []Atom{{Field: "tool_name", Matcher: mustMatcher(t, "!~^read")}}}
	if !EvalCondition(not, payload) {
		t.Fatal("not condition should match")
	}
	missing := &Condition{Mode: CombineAllOf, Atoms: []Atom{{Field: "missing", Matcher: permissionMatcherExactEmpty{}}}}
	if !EvalCondition(missing, payload) {
		t.Fatal("missing field should participate as empty string")
	}
}

type permissionMatcherExactEmpty struct{}

func (permissionMatcherExactEmpty) Match(target string, _ bool) bool { return target == "" }
func (permissionMatcherExactEmpty) String() string                   { return "=" }
