package agent

import (
	"context"
	"strings"

	"PseudoClaude/internal/worktree"
)

func (t *AgentTool) worktreePrepare(parentCWD string) AgentPrepareFunc {
	return func(ctx context.Context, runner Runner, prompt string) (Runner, string, AgentCleanupFunc, error) {
		name := worktree.RandomAgentName()
		wt, err := t.Worktrees.Create(ctx, worktree.CreateInput{Name: name, Manual: false})
		if err != nil {
			return runner, prompt, nil, err
		}
		runner.CWD = wt.Path
		runner.Env.CWD = wt.Path
		notice := buildWorktreeNotice(parentCWD, wt.Path)
		cleanup := func(ctx context.Context, result string) string {
			report, err := t.Worktrees.AutoCleanup(ctx, name)
			if err != nil {
				return strings.TrimSpace(result + "\n\nWorktree cleanup failed: " + err.Error())
			}
			if kept := worktree.FormatKept(report); kept != "" {
				return strings.TrimSpace(result + "\n\n" + kept)
			}
			return result
		}
		return runner, strings.TrimSpace(notice + "\n\n" + prompt), cleanup, nil
	}
}

func buildWorktreeNotice(parentCWD, wtPath string) string {
	return "Worktree isolation is active.\n" +
		"You are working in an independent Git worktree: " + wtPath + "\n" +
		"The parent Agent workspace is: " + parentCWD + "\n" +
		"When the parent mentions files under its workspace, map those paths to this worktree before reading or editing.\n" +
		"Before editing, re-read the target file inside this worktree. Do not write temporary results back to the parent workspace."
}
