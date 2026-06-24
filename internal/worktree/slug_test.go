package worktree

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{"alice", "team/alice", "v1.0", "a_b", "a-b", "feature/a"}
	for _, name := range valid {
		t.Run("valid_"+name, func(t *testing.T) {
			if err := ValidateName(name); err != nil {
				t.Fatalf("ValidateName(%q) = %v", name, err)
			}
		})
	}
	invalid := []string{"", strings.Repeat("a", 65), "..", ".", "../etc", "a//b", "/a", "a/", "a b", "a+b", "a/b "}
	for _, name := range invalid {
		t.Run("invalid_"+name, func(t *testing.T) {
			err := ValidateName(name)
			if !errors.Is(err, ErrInvalidName) {
				t.Fatalf("ValidateName(%q) = %v, want ErrInvalidName", name, err)
			}
		})
	}
}

func TestFlatName(t *testing.T) {
	flat, err := FlatName("team/alice")
	if err != nil {
		t.Fatal(err)
	}
	if flat != "team+alice" {
		t.Fatalf("flat = %q", flat)
	}
	if got := branchName(flat); got != "worktree-team+alice" {
		t.Fatalf("branch = %q", got)
	}
}
