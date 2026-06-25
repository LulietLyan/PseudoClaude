package backend

import (
	"errors"
	"testing"

	"PseudoClaude/internal/team"
)

func TestDetectPrefersCurrentTmux(t *testing.T) {
	got, err := Detect(FactoryDeps{
		Env: func(key string) string {
			if key == "TMUX" {
				return "/tmp/tmux"
			}
			return ""
		},
		LookPath: missingPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != team.BackendTmux {
		t.Fatalf("Detect = %q, want tmux", got)
	}
}

func TestDetectIterm2(t *testing.T) {
	got, err := Detect(FactoryDeps{
		Env: func(key string) string {
			if key == "TERM_PROGRAM" {
				return "iTerm.app"
			}
			return ""
		},
		LookPath: missingPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != team.BackendIterm2 {
		t.Fatalf("Detect = %q, want iterm2", got)
	}
}

func TestDetectTmuxCommand(t *testing.T) {
	got, err := Detect(FactoryDeps{
		Env: func(string) string { return "" },
		LookPath: func(name string) (string, error) {
			if name == "tmux" {
				return "/usr/bin/tmux", nil
			}
			return "", errors.New("missing")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != team.BackendTmux {
		t.Fatalf("Detect = %q, want tmux", got)
	}
}

func TestDetectFallsBackInProcess(t *testing.T) {
	got, err := Detect(FactoryDeps{Env: func(string) string { return "" }, LookPath: missingPath})
	if err != nil {
		t.Fatal(err)
	}
	if got != team.BackendInProcess {
		t.Fatalf("Detect = %q, want in-process", got)
	}
}

func missingPath(string) (string, error) {
	return "", errors.New("missing")
}
