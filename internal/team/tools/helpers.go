package tools

import (
	"encoding/json"
	"errors"
	"strings"

	"PseudoClaude/internal/team"
	base "PseudoClaude/internal/tools"
)

func decode(input json.RawMessage, out any) error {
	if len(input) == 0 {
		return errors.New("arguments are required")
	}
	return json.Unmarshal(input, out)
}

func jsonResult(tool string, value any) base.Result {
	data, err := json.Marshal(value)
	if err != nil {
		return base.Failure(tool, "serialization_error", err.Error(), nil)
	}
	return base.Success(tool, string(data), nil)
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func teamName(arg string, env base.Env) string {
	if strings.TrimSpace(arg) != "" {
		return strings.TrimSpace(arg)
	}
	if env.Team != nil {
		return env.Team.TeamName
	}
	return ""
}

func teamFailure(tool string, err error) base.Result {
	switch {
	case errors.Is(err, team.ErrTeamNotFound):
		return base.Failure(tool, "team_not_found", err.Error(), nil)
	case errors.Is(err, team.ErrMemberNotFound):
		return base.Failure(tool, "member_not_found", err.Error(), nil)
	case errors.Is(err, team.ErrTeamActive):
		return base.Failure(tool, "team_active", err.Error(), nil)
	default:
		return base.Failure(tool, "invalid_state", err.Error(), nil)
	}
}
