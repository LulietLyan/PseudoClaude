package permission

import "testing"

func TestMatcher(t *testing.T) {
	exact, err := CompileMatcher("=git status")
	if err != nil {
		t.Fatal(err)
	}
	if !exact.Match("git status", false) || exact.Match("git status -s", false) {
		t.Fatal("exact matcher did not match exactly")
	}

	glob, err := CompileMatcher("git *")
	if err != nil {
		t.Fatal(err)
	}
	if !glob.Match("git status", false) {
		t.Fatal("command glob should match")
	}

	path, err := CompileMatcher("**/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if !path.Match("internal/permission/rule.go", true) || path.Match("internal/permission/rule.txt", true) {
		t.Fatal("path glob should use path semantics")
	}

	regex, err := CompileMatcher("~^npm (install|test)$")
	if err != nil {
		t.Fatal(err)
	}
	if !regex.Match("npm test", false) || regex.Match("npm run test", false) {
		t.Fatal("regex matcher did not match expected targets")
	}

	not, err := CompileMatcher("!~^rm")
	if err != nil {
		t.Fatal(err)
	}
	if not.Match("rm -rf .", false) || !not.Match("ls -lh", false) {
		t.Fatal("not matcher did not invert inner matcher")
	}
}

func TestMatcherCompileErrors(t *testing.T) {
	for _, pattern := range []string{"", "=", "~[", "!", "!~["} {
		if _, err := CompileMatcher(pattern); err == nil {
			t.Fatalf("CompileMatcher(%q) expected error", pattern)
		}
	}
	if _, err := CompileMatchSpec(MatchSpec{Type: MatchRegex, Value: "["}); err == nil {
		t.Fatal("invalid structured regex should fail")
	}
	if _, err := CompileMatchSpec(MatchSpec{Type: MatchNot}); err == nil {
		t.Fatal("not without inner should fail")
	}
	if _, err := CompileMatchSpec(MatchSpec{Type: MatchKind("missing"), Value: "x"}); err == nil {
		t.Fatal("unknown matcher kind should fail")
	}
}

func TestMatchSpecEquivalentToCompactSyntax(t *testing.T) {
	spec, err := CompileMatchSpec(MatchSpec{Type: MatchNot, Inner: &MatchSpec{Type: MatchRegex, Value: "^rm"}})
	if err != nil {
		t.Fatal(err)
	}
	compact, err := CompileMatcher("!~^rm")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"rm -rf .", "ls -lh"} {
		if spec.Match(target, false) != compact.Match(target, false) {
			t.Fatalf("target %q mismatch", target)
		}
	}
}
