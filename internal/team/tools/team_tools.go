package tools

import (
	"context"
	"encoding/json"

	"PseudoClaude/internal/team"
	base "PseudoClaude/internal/tools"
)

type TeamCreateTool struct{ Manager *team.Manager }
type TeamDeleteTool struct{ Manager *team.Manager }
type TeamKillTool struct{ Manager *team.Manager }

func NewTeamCreateTool(manager *team.Manager) base.Tool { return TeamCreateTool{Manager: manager} }
func NewTeamDeleteTool(manager *team.Manager) base.Tool { return TeamDeleteTool{Manager: manager} }
func NewTeamKillTool(manager *team.Manager) base.Tool   { return TeamKillTool{Manager: manager} }

func (t TeamCreateTool) Definition() base.Definition {
	return base.Definition{
		Name:        "TeamCreate",
		Description: "Create a persistent collaboration team.",
		Safety:      base.SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"name":        stringProp("Team name."),
			"description": stringProp("Optional team description."),
			"backend":     stringProp("Optional backend override: tmux, iterm2, or in-process."),
		}, "name"),
	}
}

func (t TeamCreateTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	var args struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Backend     string `json:"backend"`
	}
	if err := decode(input, &args); err != nil {
		return base.Failure("TeamCreate", "invalid_arguments", err.Error(), nil)
	}
	leadID := ""
	if env.Team != nil {
		leadID = env.Team.AgentID
	}
	if leadID == "" {
		leadID = "lead-" + team.SanitizeName(args.Name)
	}
	created, err := t.Manager.Create(ctx, team.CreateInput{Name: args.Name, Description: args.Description, LeadAgentID: leadID, Backend: team.BackendType(args.Backend)})
	if err != nil {
		return teamFailure("TeamCreate", err)
	}
	return jsonResult("TeamCreate", map[string]any{
		"team_name":   created.Name,
		"safe_name":   created.SanitizedName,
		"backend":     created.Backend,
		"config_path": created.ConfigPath,
		"inbox_dir":   created.InboxDir,
		"tasks_path":  created.TasksPath,
	})
}

func (t TeamDeleteTool) Definition() base.Definition {
	return base.Definition{
		Name:        "TeamDelete",
		Description: "Delete a persistent collaboration team.",
		Safety:      base.SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"team_name": stringProp("Team name."),
			"force":     map[string]any{"type": "boolean", "description": "Force delete active members and state."},
		}, "team_name"),
	}
}

func (t TeamDeleteTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	var args struct {
		TeamName string `json:"team_name"`
		Force    bool   `json:"force"`
	}
	if err := decode(input, &args); err != nil {
		return base.Failure("TeamDelete", "invalid_arguments", err.Error(), nil)
	}
	if err := t.Manager.Delete(ctx, args.TeamName, args.Force); err != nil {
		return teamFailure("TeamDelete", err)
	}
	return jsonResult("TeamDelete", map[string]any{"team_name": args.TeamName, "deleted": true})
}

func (t TeamKillTool) Definition() base.Definition {
	return base.Definition{
		Name:        "TeamKill",
		Description: "Terminate and remove a team member.",
		Safety:      base.SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"team_name":   stringProp("Team name."),
			"member_name": stringProp("Member name or agent id."),
		}, "team_name", "member_name"),
	}
}

func (t TeamKillTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	var args struct {
		TeamName   string `json:"team_name"`
		MemberName string `json:"member_name"`
	}
	if err := decode(input, &args); err != nil {
		return base.Failure("TeamKill", "invalid_arguments", err.Error(), nil)
	}
	if err := t.Manager.KillMember(ctx, args.TeamName, args.MemberName); err != nil {
		return teamFailure("TeamKill", err)
	}
	return jsonResult("TeamKill", map[string]any{"team_name": args.TeamName, "member_name": args.MemberName, "killed": true})
}
