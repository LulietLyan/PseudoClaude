package backend

import (
	"os"
	"os/exec"
	"strings"

	"PseudoClaude/internal/team"
)

func Detect(deps FactoryDeps) (team.BackendType, error) {
	env := deps.Env
	if env == nil {
		env = os.Getenv
	}
	lookPath := deps.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if strings.TrimSpace(env("TMUX")) != "" {
		return team.BackendTmux, nil
	}
	if strings.TrimSpace(env("TERM_PROGRAM")) == "iTerm.app" {
		return team.BackendIterm2, nil
	}
	if _, err := lookPath("tmux"); err == nil {
		return team.BackendTmux, nil
	}
	return team.BackendInProcess, nil
}
