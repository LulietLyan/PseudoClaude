package tools

import (
	"context"
	"encoding/json"

	"PseudoClaude/internal/team"
	base "PseudoClaude/internal/tools"
)

type CompatTaskListTool struct {
	Manager *team.Manager
	Team    base.Tool
	Legacy  base.Tool
}

type CompatTaskGetTool struct {
	Manager *team.Manager
	Team    base.Tool
	Legacy  base.Tool
}

type CompatSendMessageTool struct {
	Manager *team.Manager
	Team    base.Tool
	Legacy  base.Tool
}

func NewCompatTaskListTool(manager *team.Manager, legacy base.Tool) base.Tool {
	return CompatTaskListTool{Manager: manager, Team: NewTaskListTool(manager), Legacy: legacy}
}

func NewCompatTaskGetTool(manager *team.Manager, legacy base.Tool) base.Tool {
	return CompatTaskGetTool{Manager: manager, Team: NewTaskGetTool(manager), Legacy: legacy}
}

func NewCompatSendMessageTool(manager *team.Manager, legacy base.Tool) base.Tool {
	return CompatSendMessageTool{Manager: manager, Team: NewSendMessageTool(manager), Legacy: legacy}
}

func (t CompatTaskListTool) Definition() base.Definition {
	def := t.Legacy.Definition()
	def.Description = "List background tasks, or shared team tasks when team_name/current team context is provided."
	def.InputSchema = objectSchema(map[string]any{
		"team_name": stringProp("Optional team name for shared team tasks."),
		"status":    stringProp("Optional team task status filter."),
	})
	return def
}

func (t CompatTaskListTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	if wantsTeam(input, env) {
		return t.Team.Execute(ctx, input, env)
	}
	return t.Legacy.Execute(ctx, input, env)
}

func (t CompatTaskGetTool) Definition() base.Definition {
	def := t.Legacy.Definition()
	def.Description = "Get a background task, or a shared team task when team_name/current team context is provided."
	def.InputSchema = objectSchema(map[string]any{
		"team_name": stringProp("Optional team name for shared team tasks."),
		"task_id":   stringProp("Task id."),
	}, "task_id")
	return def
}

func (t CompatTaskGetTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	if wantsTeam(input, env) {
		return t.Team.Execute(ctx, input, env)
	}
	return t.Legacy.Execute(ctx, input, env)
}

func (t CompatSendMessageTool) Definition() base.Definition {
	def := t.Legacy.Definition()
	def.Description = "Send a message to a completed named background Agent, or to a team recipient when team_name/current team context is provided."
	def.InputSchema = objectSchema(map[string]any{
		"team_name": stringProp("Optional team name for team messaging."),
		"name":      stringProp("Legacy named background Agent."),
		"message":   stringProp("Legacy follow-up message."),
		"to":        stringProp("Team recipient member name, agent id, lead, or broadcast."),
		"type":      stringProp("Team message type."),
		"summary":   stringProp("Team message summary."),
		"content":   stringProp("Team message body."),
		"payload":   map[string]any{"type": "object", "additionalProperties": true},
	})
	return def
}

func (t CompatSendMessageTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	if wantsTeam(input, env) || hasField(input, "to") {
		return t.Team.Execute(ctx, input, env)
	}
	if teamInput, ok := t.legacyMessageToTeam(input); ok {
		return t.Team.Execute(ctx, teamInput, env)
	}
	return t.Legacy.Execute(ctx, input, env)
}

func (t CompatSendMessageTool) legacyMessageToTeam(input json.RawMessage) (json.RawMessage, bool) {
	var args struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(input, &args); err != nil || args.Name == "" || args.Message == "" {
		return nil, false
	}
	for _, tm := range t.teamNames() {
		if _, err := t.Manager.ResolveMember(tm, args.Name); err == nil {
			data, err := json.Marshal(map[string]any{
				"team_name": tm,
				"to":        args.Name,
				"summary":   "follow-up",
				"content":   args.Message,
			})
			return data, err == nil
		}
	}
	return nil, false
}

func (t CompatSendMessageTool) teamNames() []string {
	if t.Manager == nil {
		return nil
	}
	teams := t.Manager.List()
	out := make([]string, 0, len(teams))
	for _, tm := range teams {
		out = append(out, tm.Name)
		if tm.SanitizedName != tm.Name {
			out = append(out, tm.SanitizedName)
		}
	}
	return out
}

func wantsTeam(input json.RawMessage, env base.Env) bool {
	if env.Team != nil && env.Team.TeamName != "" {
		return true
	}
	return hasField(input, "team_name")
}

func hasField(input json.RawMessage, field string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input, &raw); err != nil {
		return false
	}
	value, ok := raw[field]
	if !ok {
		return false
	}
	var s string
	if err := json.Unmarshal(value, &s); err == nil {
		return s != ""
	}
	return true
}
