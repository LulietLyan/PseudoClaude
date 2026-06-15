package prompt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const gitStatusTimeout = 300 * time.Millisecond

type Environment struct {
	WorkingDir string
	Platform   string
	Date       string
	GitStatus  string
	Version    string
	Provider   string
	Model      string
}

func GatherEnvironment(version, provider, model, cwd string) Environment {
	wd := strings.TrimSpace(cwd)
	if wd == "" {
		if current, err := os.Getwd(); err == nil {
			wd = current
		}
	}
	env := Environment{
		WorkingDir: wd,
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
		Date:       time.Now().Format("2006-01-02"),
		GitStatus:  gatherGitStatus(wd),
		Version:    strings.TrimSpace(version),
		Provider:   strings.TrimSpace(provider),
		Model:      strings.TrimSpace(model),
	}
	return env
}

func (e Environment) Render() string {
	lines := []string{"Runtime environment:"}
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			value = "unavailable"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", label, value))
	}
	add("working directory", e.WorkingDir)
	add("platform", e.Platform)
	add("date", e.Date)
	add("git status", e.GitStatus)
	add("version", e.Version)
	add("provider", e.Provider)
	add("model", e.Model)
	return strings.Join(lines, "\n")
}

func gatherGitStatus(cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitStatusTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return "clean"
	}
	return fmt.Sprintf("%d changed files", len(lines))
}
