package worktree

import "fmt"

func FormatKept(report *AutoCleanupReport) string {
	if report == nil || !report.Kept {
		return ""
	}
	return fmt.Sprintf("Worktree retained for review: %s\nBranch: %s\nReason: %s", report.Path, report.Branch, report.Reason)
}
