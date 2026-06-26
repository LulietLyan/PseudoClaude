package tools

import (
	"context"
	"encoding/json"
	"strings"

	"PseudoClaude/internal/team"
	"PseudoClaude/internal/team/mailbox"
	base "PseudoClaude/internal/tools"
)

type SendMessageTool struct{ Manager *team.Manager }

func NewSendMessageTool(manager *team.Manager) base.Tool { return SendMessageTool{Manager: manager} }

func (t SendMessageTool) Definition() base.Definition {
	return base.Definition{
		Name:        "SendMessage",
		Description: "Send a message to a teammate, the Lead, or every teammate in the current team.",
		Safety:      base.SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"team_name": stringProp("Optional team name. Defaults to current team context."),
			"to":        stringProp("Recipient member name, agent id, lead, or broadcast."),
			"type":      stringProp("Message type: text, shutdown_request, shutdown_response, plan_approval_response."),
			"summary":   stringProp("Short summary."),
			"content":   stringProp("Message body."),
			"payload":   map[string]any{"type": "object", "additionalProperties": true},
		}, "to", "summary"),
	}
}

func (t SendMessageTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	var args struct {
		TeamName string         `json:"team_name"`
		To       string         `json:"to"`
		Type     string         `json:"type"`
		Summary  string         `json:"summary"`
		Content  string         `json:"content"`
		Payload  map[string]any `json:"payload"`
	}
	if err := decode(input, &args); err != nil {
		return base.Failure("SendMessage", "invalid_arguments", err.Error(), nil)
	}
	name := teamName(args.TeamName, env)
	if name == "" {
		return base.Failure("SendMessage", "invalid_arguments", "team_name is required outside team context", nil)
	}
	msgType := mailbox.MessageType(args.Type)
	if msgType == "" {
		msgType = mailbox.MessageText
	}
	if msgType == mailbox.MessagePlanApprovalResponse && (env.Team == nil || !env.Team.IsLead) {
		return base.Failure("SendMessage", "forbidden", "only the Lead can send plan_approval_response", nil)
	}
	from := "lead"
	if env.Team != nil {
		from = env.Team.AgentID
		if env.Team.MemberName != "" {
			from = env.Team.MemberName
		}
	}
	store, err := t.Manager.MailboxStore(name)
	if err != nil {
		return teamFailure("SendMessage", err)
	}
	recipients, err := t.recipients(name, args.To, env)
	if err != nil {
		return teamFailure("SendMessage", err)
	}
	resumed := map[string]string{}
	for _, recipient := range recipients {
		msg := mailbox.Message{From: from, To: recipient.AgentID, Type: msgType, Summary: args.Summary, Content: args.Content, Payload: args.Payload}
		if err := store.Write(ctx, recipient.AgentID, msg); err != nil {
			return teamFailure("SendMessage", err)
		}
		if shouldResume(recipient) {
			taskID, err := t.Manager.ResumeMember(ctx, name, recipient.AgentID)
			if err != nil {
				return teamFailure("SendMessage", err)
			}
			resumed[recipient.Name] = taskID
		}
	}
	payload := map[string]any{"team_name": name, "recipients": memberNames(recipients), "type": msgType}
	if len(resumed) > 0 {
		payload["resumed"] = resumed
	}
	return jsonResult("SendMessage", payload)
}

func (t SendMessageTool) recipients(teamName, to string, env base.Env) ([]team.MemberInfo, error) {
	to = strings.TrimSpace(to)
	if to == "broadcast" || to == "*" || to == "all" {
		members, err := t.Manager.TeamMembers(teamName)
		if err != nil {
			return nil, err
		}
		out := []team.MemberInfo{}
		fromAgent := ""
		if env.Team != nil {
			fromAgent = env.Team.AgentID
		}
		for _, member := range members {
			if member.AgentID != "" && member.AgentID != fromAgent {
				out = append(out, member)
			}
		}
		return out, nil
	}
	member, err := t.Manager.ResolveMember(teamName, to)
	if err != nil {
		return nil, err
	}
	return []team.MemberInfo{member}, nil
}

func memberNames(members []team.MemberInfo) []string {
	out := make([]string, 0, len(members))
	for _, member := range members {
		out = append(out, member.Name)
	}
	return out
}

func shouldResume(member team.MemberInfo) bool {
	return member.BackendType == team.BackendInProcess && member.IsActive != nil && !*member.IsActive
}
