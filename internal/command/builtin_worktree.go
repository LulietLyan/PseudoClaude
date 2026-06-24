package command

import (
	"fmt"
	"strings"
)

func handleWorktreeCommand(ctx Context, ctl Controller) error {
	worktrees, ok := ctl.(WorktreeController)
	if !ok {
		ctl.Show(MessageError, "Worktree 功能不可用：当前界面未接入 Worktree 管理器。")
		return nil
	}
	args := strings.Fields(ctx.Args)
	if !worktrees.WorktreeAvailable() {
		ctl.Show(MessageError, "Worktree 功能不可用：当前目录不是可用的 Git worktree 仓库。")
		return nil
	}
	if len(args) == 0 || args[0] == "list" {
		ctl.Show(MessageInfo, FormatWorktrees(worktrees.ListWorktrees()))
		return nil
	}
	switch args[0] {
	case "create":
		if len(args) != 2 {
			ctl.Show(MessageHelp, "Usage: /worktree create <name>")
			return nil
		}
		summary, err := worktrees.CreateWorktree(args[1])
		showWorktreeResult(ctl, "Created", summary, err)
	case "enter":
		if len(args) != 2 {
			ctl.Show(MessageHelp, "Usage: /worktree enter <name>")
			return nil
		}
		summary, err := worktrees.EnterWorktree(args[1])
		showWorktreeResult(ctl, "Entered", summary, err)
	case "exit":
		remove, discard := parseWorktreeFlags(args[1:])
		summary, err := worktrees.ExitWorktree(remove, discard)
		showWorktreeResult(ctl, "Exited", summary, err)
	case "remove":
		if len(args) < 2 {
			ctl.Show(MessageHelp, "Usage: /worktree remove <name> [--discard]")
			return nil
		}
		_, discard := parseWorktreeFlags(args[2:])
		summary, err := worktrees.RemoveWorktree(args[1], discard)
		showWorktreeResult(ctl, "Removed", summary, err)
	default:
		ctl.Show(MessageHelp, "Usage: /worktree [list|create <name>|enter <name>|exit [--remove] [--discard]|remove <name> [--discard]]")
	}
	return nil
}

func parseWorktreeFlags(args []string) (remove bool, discard bool) {
	for _, arg := range args {
		switch arg {
		case "--remove":
			remove = true
		case "--discard":
			discard = true
		}
	}
	return remove, discard
}

func showWorktreeResult(ctl Controller, action string, summary WorktreeSummary, err error) {
	if err != nil {
		ctl.Show(MessageError, fmt.Sprintf("%s worktree failed: %v", strings.ToLower(action), err))
		return
	}
	ctl.Show(MessageInfo, action+" worktree:\n"+FormatWorktree(summary))
	ctl.RefreshStatus()
}
