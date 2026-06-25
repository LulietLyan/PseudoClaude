package command

import (
	"fmt"
	"strings"
)

func handleTeamCommand(ctx Context, ctl Controller) error {
	teamCtl, ok := ctl.(TeamController)
	if !ok || !teamCtl.TeamAvailable() {
		ctl.Show(MessageError, "Team manager is unavailable.")
		return nil
	}
	args := strings.Fields(ctx.Args)
	if len(args) == 0 || args[0] == "list" {
		ctl.Show(MessageInfo, FormatTeams(teamCtl.ListTeams()))
		return nil
	}
	switch args[0] {
	case "info":
		if len(args) < 2 {
			ctl.Show(MessageError, "Usage: /team info <name>")
			return nil
		}
		detail, ok := teamCtl.TeamInfo(args[1])
		if !ok {
			ctl.Show(MessageError, "Team not found: "+args[1])
			return nil
		}
		ctl.Show(MessageInfo, FormatTeamDetail(detail))
	case "delete":
		if len(args) < 2 {
			ctl.Show(MessageError, "Usage: /team delete <name> [--force]")
			return nil
		}
		force := hasArg(args[2:], "--force")
		if err := teamCtl.DeleteTeam(args[1], force); err != nil {
			ctl.Show(MessageError, err.Error())
			return nil
		}
		ctl.Show(MessageInfo, fmt.Sprintf("Deleted team %s.", args[1]))
	case "kill":
		if len(args) < 3 {
			ctl.Show(MessageError, "Usage: /team kill <team> <member>")
			return nil
		}
		if err := teamCtl.KillTeamMember(args[1], args[2]); err != nil {
			ctl.Show(MessageError, err.Error())
			return nil
		}
		ctl.Show(MessageInfo, fmt.Sprintf("Killed member %s in team %s.", args[2], args[1]))
	default:
		ctl.Show(MessageError, "Unknown /team action: "+args[0])
	}
	return nil
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func FormatTeams(teams []TeamSummary) string {
	if len(teams) == 0 {
		return "No teams."
	}
	var b strings.Builder
	b.WriteString("Teams:\n")
	for _, team := range teams {
		b.WriteString(fmt.Sprintf("- %s (%s) backend=%s members=%d active=%d\n", team.Name, team.SanitizedName, team.Backend, team.MemberCount, team.ActiveCount))
	}
	return strings.TrimSpace(b.String())
}

func FormatTeamDetail(detail TeamDetail) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Team: %s (%s)\n", detail.Name, detail.SanitizedName))
	b.WriteString(fmt.Sprintf("Backend: %s\n", detail.Backend))
	b.WriteString(fmt.Sprintf("Config: %s\n", detail.ConfigPath))
	b.WriteString(fmt.Sprintf("Inboxes: %s\n", detail.InboxDir))
	b.WriteString(fmt.Sprintf("Tasks: %s\n", detail.TasksPath))
	if len(detail.Members) == 0 {
		b.WriteString("Members: none\n")
		return strings.TrimSpace(b.String())
	}
	b.WriteString("Members:\n")
	for _, member := range detail.Members {
		b.WriteString(fmt.Sprintf("- %s agent=%s active=%s backend=%s worktree=%s\n", member.Name, member.AgentID, member.Active, member.Backend, member.WorktreePath))
	}
	return strings.TrimSpace(b.String())
}
