package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/subagent"
	"PseudoClaude/internal/tools"
)

type ProviderResolver func(model string, parent RunnerSnapshot) (llm.Provider, error)

type BackgroundPolicy struct {
	ForegroundTimeout time.Duration
}

type AgentTool struct {
	Catalog    *subagent.Catalog
	Tasks      AgentTaskLauncher
	Parent     *RunnerHandle
	Providers  ProviderResolver
	Background BackgroundPolicy
}

type AgentTaskLauncher interface {
	LaunchAgent(ctx context.Context, in AgentLaunchInput) (string, error)
}

type AgentLaunchInput struct {
	Name         string
	Type         string
	Fork         bool
	Prompt       string
	Runner       Runner
	Conversation *conversation.Conversation
}

type AgentToolInput struct {
	Prompt          string `json:"prompt"`
	Description     string `json:"description"`
	SubagentType    string `json:"subagent_type,omitempty"`
	Model           string `json:"model,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
	Name            string `json:"name,omitempty"`
}

func (t *AgentTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "Agent",
		Description: t.description(),
		Safety:      tools.SafetySideEffect,
		Timeout:     5 * time.Minute,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":            map[string]any{"type": "string", "description": "Task instructions for the sub Agent."},
				"description":       map[string]any{"type": "string", "description": "Short task description."},
				"subagent_type":     map[string]any{"type": "string", "description": "Optional registered sub Agent role. Omit to fork the current conversation."},
				"model":             map[string]any{"type": "string", "description": "Optional model override."},
				"run_in_background": map[string]any{"type": "boolean", "description": "Run asynchronously in the background."},
				"name":              map[string]any{"type": "string", "description": "Optional name for a background sub Agent."},
			},
			"required": []string{"prompt", "description"},
		},
	}
}

func (t *AgentTool) Execute(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	var args AgentToolInput
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Failure("Agent", "invalid_arguments", err.Error(), nil)
	}
	args.Prompt = strings.TrimSpace(args.Prompt)
	args.Description = strings.TrimSpace(args.Description)
	args.SubagentType = strings.TrimSpace(args.SubagentType)
	if args.Prompt == "" || args.Description == "" {
		return tools.Failure("Agent", "invalid_arguments", "prompt and description are required", nil)
	}
	parent := t.Parent.Snapshot()
	if parent.Registry == nil || parent.Provider == nil || parent.Conversation == nil {
		return tools.Failure("Agent", "not_ready", "parent runner context is not ready", nil)
	}
	if parentRunnerIsSubagent(parent) {
		if parentRunnerIsFork(parent) {
			return tools.Failure("Agent", "nested_agent_forbidden", "Fork sub Agents cannot start another Agent", nil)
		}
		return tools.Failure("Agent", "nested_agent_forbidden", "Sub Agents cannot start another Agent", nil)
	}

	if args.SubagentType == "" {
		return t.executeFork(ctx, args, parent)
	}
	def, ok := t.Catalog.Resolve(args.SubagentType)
	if !ok {
		return tools.Failure("Agent", "unknown_subagent_type", "unknown subagent_type: "+args.SubagentType, map[string]any{"subagent_type": args.SubagentType})
	}
	return t.executeDefined(ctx, args, parent, def)
}

func (t *AgentTool) executeDefined(ctx context.Context, args AgentToolInput, parent RunnerSnapshot, def subagent.Definition) tools.Result {
	background := args.RunInBackground || def.Background
	runner := t.childRunner(parent, def, false, background)
	childConv := &conversation.Conversation{}
	if background {
		return t.launch(ctx, args, def, runner, childConv, false, "async_launched")
	}
	timeout := t.Background.ForegroundTimeout
	if timeout > 0 {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		result := runner.RunToCompletion(runCtx, RunToCompletionInput{
			Request:  Request{Conversation: childConv},
			TaskText: args.Prompt,
		})
		if result.Stop.Reason == StopCanceled && runCtx.Err() == context.DeadlineExceeded {
			return t.launch(ctx, args, def, runner, childConv, false, "timed_out_to_background")
		}
		return completionToolResult(def.Name, result)
	}
	result := runner.RunToCompletion(ctx, RunToCompletionInput{
		Request:  Request{Conversation: childConv},
		TaskText: args.Prompt,
	})
	return completionToolResult(def.Name, result)
}

func (t *AgentTool) executeFork(ctx context.Context, args AgentToolInput, parent RunnerSnapshot) tools.Result {
	def := subagent.ForkDefinition()
	messages := subagent.BuildForkMessages(parent.Conversation.Messages(), args.Prompt)
	childConv := conversation.NewFromMessages(messages, conversation.Hooks{})
	runner := t.childRunner(parent, def, true, true)
	return t.launch(ctx, args, def, runner, childConv, true, "async_launched")
}

func (t *AgentTool) childRunner(parent RunnerSnapshot, def subagent.Definition, fork bool, background bool) Runner {
	provider := parent.Provider
	if t.Providers != nil {
		if resolved, err := t.Providers(string(def.Model), parent); err == nil && resolved != nil {
			provider = resolved
		}
	}
	allowed := tools.FilterSubAgentTools(parent.Registry, tools.FilterPolicy{
		DefinitionTools:      def.Tools,
		DefinitionDisallowed: def.DisallowedTools,
		Background:           background,
		Fork:                 fork,
	})
	if def.Permission == subagent.PermissionPlan {
		allowed = readOnlyToolNames(parent.Registry, allowed)
	}
	upgrader := parent.Approval
	if background {
		upgrader = nil
	}
	if upgrader == nil {
		upgrader = func(ctx context.Context, req ApprovalRequest) (permission.ApprovalDecision, error) {
			return permission.ApprovalDenyOnce, nil
		}
	}
	return Runner{
		Provider:      provider,
		Registry:      parent.Registry,
		Env:           parent.Env,
		Config:        parent.Config,
		Version:       parent.Version,
		Permission:    parent.Permission,
		Instructions:  parent.Instructions,
		AllowedTools:  allowed,
		Hooks:         parent.Hooks,
		SessionID:     parent.SessionID,
		CWD:           parent.CWD,
		SkillsCatalog: parent.SkillsCatalog,
		ActiveSkills:  parent.ActiveSkills,
		Sub: SubRunOptions{
			SystemPrompt:     def.SystemPrompt,
			MaxTurns:         def.MaxTurns,
			PermissionMode:   permissionModeFromRef(def.Permission, parent.PermissionMode),
			DontAsk:          true,
			IsSubAgent:       true,
			IsFork:           fork,
			ParentLabel:      def.Name,
			ApprovalUpgrader: upgrader,
		},
	}
}

func (t *AgentTool) launch(ctx context.Context, args AgentToolInput, def subagent.Definition, runner Runner, conv *conversation.Conversation, fork bool, status string) tools.Result {
	id, err := t.Tasks.LaunchAgent(ctx, AgentLaunchInput{
		Name:         args.Name,
		Type:         def.Name,
		Fork:         fork,
		Prompt:       args.Prompt,
		Runner:       runner,
		Conversation: conv,
	})
	if err != nil {
		return tools.Failure("Agent", "launch_failed", err.Error(), nil)
	}
	return jsonSuccess("Agent", map[string]any{"task_id": id, "status": status, "subagent_type": def.Name})
}

func (t *AgentTool) description() string {
	var b strings.Builder
	b.WriteString("Start a focused sub Agent. Provide subagent_type for a registered role, or omit it to fork the current conversation into a background task.")
	if t != nil && t.Catalog != nil {
		defs := t.Catalog.List()
		if len(defs) > 0 {
			b.WriteString(" Available roles:")
			for _, def := range defs {
				b.WriteString(" ")
				b.WriteString(def.Name)
				b.WriteString(" - ")
				b.WriteString(def.Description)
				b.WriteString(";")
			}
		}
	}
	return b.String()
}

func completionToolResult(name string, result CompletionResult) tools.Result {
	content := result.Text
	if result.Stop.Reason != StopCompleted {
		content = strings.TrimSpace(content)
		if content != "" {
			content += "\n\n"
		}
		content += fmt.Sprintf("Sub Agent stopped with %s: %s", result.Stop.Reason, result.Stop.Message)
	}
	return tools.Success("Agent", content, map[string]any{
		"subagent_type": name,
		"stop_reason":   string(result.Stop.Reason),
		"tool_count":    result.ToolCount,
	})
}

func permissionModeFromRef(ref subagent.PermissionRef, inherited permission.Mode) permission.Mode {
	switch ref {
	case subagent.PermissionStrict:
		return permission.ModeStrict
	case subagent.PermissionPlan:
		return permission.ModeDefault
	case subagent.PermissionDefault:
		return permission.ModeDefault
	case subagent.PermissionAcceptEdits:
		return permission.ModeAcceptEdits
	case subagent.PermissionBypassPermissions:
		return permission.ModeBypassPermissions
	default:
		return inherited
	}
}

func readOnlyToolNames(reg *tools.Registry, names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if safety, ok := reg.Safety(name); ok && safety == tools.SafetyReadOnly {
			out = append(out, name)
		}
	}
	return out
}

func jsonSuccess(tool string, value any) tools.Result {
	data, err := json.Marshal(value)
	if err != nil {
		return tools.Failure(tool, "serialization_error", err.Error(), nil)
	}
	return tools.Success(tool, string(data), nil)
}

func parentRunnerIsSubagent(parent RunnerSnapshot) bool {
	return parent.Sub.IsSubAgent
}

func parentRunnerIsFork(parent RunnerSnapshot) bool {
	return parent.Sub.IsFork
}
