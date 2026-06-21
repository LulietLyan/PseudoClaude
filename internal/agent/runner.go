package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"PseudoClaude/internal/compact"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/hook"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/memory"
	"PseudoClaude/internal/permission"
	"PseudoClaude/internal/prompt"
	"PseudoClaude/internal/tools"
)

const planReminderInterval = 4

type Runner struct {
	Provider      llm.Provider
	Registry      *tools.Registry
	Env           tools.Env
	Config        Config
	Version       string
	Permission    *permission.Engine
	Compact       *compact.Runtime
	Instructions  string
	Memory        MemoryUpdater
	SkillsCatalog func() []prompt.SkillCatalogItem
	ActiveSkills  func() []prompt.ActiveSkillEntry
	AllowedTools  []string
	Hooks         *hook.Engine
	HookPrompts   *hook.PromptQueue
	SessionID     string
	CWD           string
}

type MemoryUpdater interface {
	IndexText() string
	UpdateAsync(context.Context, memory.UpdateInput)
}

type Config struct {
	MaxIterations       int
	MaxUnknownToolCalls int
}

type Request struct {
	Mode           Mode
	UserText       string
	PlanTask       string
	PlanText       string
	PermissionMode permission.Mode
	Conversation   *conversation.Conversation
}

func DefaultConfig() Config {
	return Config{MaxIterations: 15, MaxUnknownToolCalls: 2}
}

func (c Config) normalize() Config {
	defaults := DefaultConfig()
	if c.MaxIterations <= 0 {
		c.MaxIterations = defaults.MaxIterations
	}
	if c.MaxUnknownToolCalls <= 0 {
		c.MaxUnknownToolCalls = defaults.MaxUnknownToolCalls
	}
	return c
}

func (r Runner) Run(ctx context.Context, req Request) <-chan Event {
	events := make(chan Event)
	go func() {
		defer close(events)
		r.run(ctx, req, events)
	}()
	return events
}

func (r Runner) run(ctx context.Context, req Request, events chan<- Event) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := r.Config.normalize()
	if req.Conversation == nil {
		req.Conversation = &conversation.Conversation{}
	}
	if r.Registry == nil {
		r.Registry, _ = tools.NewRegistry()
	}
	if r.Provider == nil {
		err := errors.New("provider is nil")
		sendEvent(ctx, events, Event{Type: EventError, Err: err})
		sendStop(ctx, events, 0, StopStreamError, "provider is nil")
		return
	}

	userText, defs, toolOpts := r.prepareRequest(req)
	startLen := req.Conversation.Len()
	permissionMode := req.PermissionMode
	if permissionMode == "" {
		if r.Permission != nil {
			permissionMode = r.Permission.StartMode()
		} else {
			permissionMode = permission.ModeDefault
		}
	}
	req.Conversation.AddUser(userText)
	memoryIndex := ""
	if r.Memory != nil {
		memoryIndex = r.Memory.IndexText()
	}
	stableSystem := prompt.BuildSystemPrompt(prompt.PromptInputs{
		Instructions:  r.Instructions,
		SkillsCatalog: prompt.RenderSkillsCatalog(r.skillCatalogItems()),
		Memory:        memoryIndex,
	})
	sendEvent(ctx, events, Event{Type: EventProgress, Message: "agent run started"})

	unknownCount := 0
	for iteration := 1; ; iteration++ {
		if ctx.Err() != nil {
			sendStop(context.Background(), events, iteration-1, StopCanceled, "canceled")
			return
		}
		if iteration > cfg.MaxIterations {
			sendStop(ctx, events, iteration-1, StopMaxIterations, "reached maximum iterations")
			return
		}
		sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: "requesting model"})

		if r.Compact != nil {
			r.dispatchHook(ctx, hook.EventPreCompact, permissionMode, hook.Payload{"trigger": "auto"})
			out, err := compact.ManageContext(ctx, compact.ManageInput{
				Conversation: req.Conversation,
				Runtime:      r.Compact,
				Provider:     r.Provider,
				Trigger:      compact.TriggerAuto,
				OnProgress: func(message string) {
					sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: message})
				},
			})
			if out.TriggeredLayer1 {
				sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: fmt.Sprintf("工具结果已落盘：%d 个；estimated tokens %d -> %d", out.OffloadedCount, out.BeforeTokens, out.AfterTokens)})
			}
			if out.TriggeredLayer2 {
				sendEvent(ctx, events, Event{Type: EventProgress, Iteration: iteration, Message: fmt.Sprintf("上下文已压缩：estimated tokens %d -> %d", out.BeforeTokens, out.AfterTokens)})
			}
			r.dispatchHook(ctx, hook.EventPostCompact, permissionMode, hook.Payload{
				"trigger":       "auto",
				"before_tokens": out.BeforeTokens,
				"after_tokens":  out.AfterTokens,
			})
			if err != nil {
				sendEvent(ctx, events, Event{Type: EventError, Iteration: iteration, Err: err})
				sendStop(ctx, events, iteration, StopStreamError, err.Error())
				return
			}
		}

		collector := &streamCollector{}
		if result := r.dispatchHook(ctx, hook.EventPreUserMessage, permissionMode, hook.Payload{"prompt": lastUserPrompt(req.Conversation)}); len(result.InjectedPrompts) > 0 && r.HookPrompts != nil {
			r.HookPrompts.Add(result.InjectedPrompts...)
		}
		environment := prompt.GatherEnvironment(r.Version, r.Provider.Name(), r.Provider.Model(), r.Env.CWD).Render()
		if active := prompt.RenderActiveSkills(r.activeSkillEntries()); active != "" {
			environment = strings.TrimSpace(environment) + "\n\n" + active
		}
		modelReq := llm.Request{
			Messages: req.Conversation.Messages(),
			Tools:    defs,
			System: llm.System{
				Stable:      stableSystem,
				Environment: environment,
			},
			Reminder: r.reminder(req.Mode, iteration),
		}
		out, err := collector.collect(ctx, iteration, r.Provider.Stream(ctx, modelReq), events)
		if err != nil {
			if ctx.Err() != nil {
				sendStop(context.Background(), events, iteration, StopCanceled, "canceled")
				return
			}
			r.dispatchHook(ctx, hook.EventNotification, permissionMode, hook.Payload{"kind": "stream_error", "detail": err.Error()})
			sendEvent(ctx, events, Event{Type: EventError, Iteration: iteration, Err: err})
			sendStop(ctx, events, iteration, StopStreamError, err.Error())
			return
		}
		if strings.TrimSpace(out.Text) != "" {
			req.Conversation.AddAssistant(out.Text)
			sendEvent(ctx, events, Event{Type: EventAssistantText, Iteration: iteration, Text: out.Text, Message: out.Text})
		}
		if len(out.ToolCalls) == 0 {
			if r.Compact != nil {
				r.Compact.UpdateUsageAnchor(out.Usage, req.Conversation.Len())
			}
			unknownCount = 0
			r.updateMemoryAfterRun(req.Conversation, startLen)
			r.dispatchHook(ctx, hook.EventStop, permissionMode, hook.Payload{"iter": iteration})
			sendStop(ctx, events, iteration, StopCompleted, "completed")
			return
		}

		req.Conversation.AddAssistantToolCalls(out.ToolCalls)
		if r.Compact != nil {
			r.Compact.UpdateUsageAnchor(out.Usage, req.Conversation.Len())
		}
		hooks := toolHookContext{
			Engine:    r.Hooks,
			SessionID: r.sessionID(),
			CWD:       r.cwd(),
			Mode:      permissionMode,
		}
		results, err := executeToolCalls(ctx, r.Registry, r.Env, iteration, out.ToolCalls, events, toolOpts, r.Permission, permissionMode, hooks)
		for _, result := range results {
			req.Conversation.AddToolResult(llm.ToolResult{
				CallID:  result.Call.ID,
				Name:    result.Call.Name,
				Content: result.Result.JSON(),
				IsError: !result.Result.OK,
			})
			if r.Registry.IsKnown(result.Call.Name) {
				unknownCount = 0
			} else if result.Result.ErrorType == "unknown_tool" {
				unknownCount++
			}
		}
		if err != nil {
			sendStop(context.Background(), events, iteration, StopCanceled, "canceled")
			return
		}
		if unknownCount >= cfg.MaxUnknownToolCalls {
			sendStop(ctx, events, iteration, StopUnknownToolLimit, "too many unknown tool calls")
			return
		}
	}
}

func (r Runner) dispatchHook(ctx context.Context, event hook.Event, mode permission.Mode, extra hook.Payload) hook.DispatchResult {
	if r.Hooks == nil {
		return hook.DispatchResult{}
	}
	payload := hook.NewPayload(event, r.sessionID(), r.cwd(), mode)
	for key, value := range extra {
		payload[key] = value
	}
	return r.Hooks.Dispatch(ctx, event, payload)
}

func (r Runner) sessionID() string {
	if r.SessionID != "" {
		return r.SessionID
	}
	if r.Compact != nil {
		return r.Compact.Snapshot().Session.ID
	}
	return ""
}

func (r Runner) cwd() string {
	if r.CWD != "" {
		return r.CWD
	}
	return r.Env.CWD
}

func (r Runner) reminder(mode Mode, iteration int) string {
	parts := []string{}
	if base := strings.TrimSpace(reminderForMode(mode, iteration)); base != "" {
		parts = append(parts, base)
	}
	if r.HookPrompts != nil {
		parts = append(parts, r.HookPrompts.Drain()...)
	}
	return strings.Join(parts, "\n\n")
}

func lastUserPrompt(conv *conversation.Conversation) string {
	if conv == nil {
		return ""
	}
	messages := conv.Messages()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

func (r Runner) updateMemoryAfterRun(conv *conversation.Conversation, startLen int) {
	if r.Memory == nil || conv == nil {
		return
	}
	messages := conv.Messages()
	if startLen < 0 || startLen >= len(messages) {
		startLen = 0
	}
	r.Memory.UpdateAsync(context.Background(), memory.UpdateInput{Messages: messages[startLen:]})
}

func (r Runner) prepareRequest(req Request) (string, []tools.Definition, toolExecutionOptions) {
	if r.Registry == nil {
		return requestText(req), nil, toolExecutionOptions{}
	}
	allowedNames := allowedNameSet(r.AllowedTools)
	switch req.Mode {
	case ModePlan:
		return planPrompt(req.PlanTask), filterDefinitionsBySafety(r.Registry.DefinitionsFiltered(r.AllowedTools), tools.SafetyReadOnly), toolExecutionOptions{
			AllowedSafety: map[tools.Safety]bool{tools.SafetyReadOnly: true},
			AllowedNames:  allowedNames,
		}
	case ModeDo:
		return doPrompt(req.PlanTask, req.PlanText), r.Registry.DefinitionsFiltered(r.AllowedTools), toolExecutionOptions{AllowedNames: allowedNames}
	default:
		return requestText(req), r.Registry.DefinitionsFiltered(r.AllowedTools), toolExecutionOptions{AllowedNames: allowedNames}
	}
}

func (r Runner) skillCatalogItems() []prompt.SkillCatalogItem {
	if r.SkillsCatalog == nil {
		return nil
	}
	return r.SkillsCatalog()
}

func (r Runner) activeSkillEntries() []prompt.ActiveSkillEntry {
	if r.ActiveSkills == nil {
		return nil
	}
	return r.ActiveSkills()
}

func allowedNameSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]bool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func filterDefinitionsBySafety(defs []tools.Definition, safety tools.Safety) []tools.Definition {
	out := make([]tools.Definition, 0, len(defs))
	for _, def := range defs {
		if def.Safety == safety {
			out = append(out, def)
		}
	}
	return out
}

func requestText(req Request) string {
	if strings.TrimSpace(req.UserText) != "" {
		return req.UserText
	}
	if strings.TrimSpace(req.PlanTask) != "" {
		return req.PlanTask
	}
	return strings.TrimSpace(req.PlanText)
}

func planPrompt(task string) string {
	return fmt.Sprintf(`Plan mode. Your job is to clarify and plan before implementation.

Rules:
- Use only read-only tools to inspect the workspace.
- Do not edit files, create directories, run commands, install dependencies, or make any project changes.
- If the task is broad or underspecified, ask concise clarifying questions first instead of inventing requirements.
- If you already have enough information, produce an implementation plan with target files, steps, validation, and risks.
- Do not claim that files or directories were created in Plan mode.

Task:
%s`, strings.TrimSpace(task))
}

func doPrompt(task, plan string) string {
	return fmt.Sprintf("Execution mode. Carry out the following task using the approved plan. Use the conversation history and tool results as context.\n\nOriginal task:\n%s\n\nPlan:\n%s", strings.TrimSpace(task), strings.TrimSpace(plan))
}

func reminderForMode(mode Mode, iteration int) string {
	if mode != ModePlan {
		return ""
	}
	full := iteration == 1 || (iteration-1)%planReminderInterval == 0
	return prompt.PlanReminder(full)
}

func sendStop(ctx context.Context, events chan<- Event, iteration int, reason StopReason, message string) {
	sendEvent(ctx, events, Event{
		Type:      EventStop,
		Iteration: iteration,
		Stop: &Stop{
			Reason:     reason,
			Message:    message,
			Iterations: iteration,
		},
	})
}
