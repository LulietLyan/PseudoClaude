package task

import (
	"context"
	"encoding/json"
	"errors"

	"PseudoClaude/internal/tools"
)

func NewTaskListTool(manager *Manager) tools.Tool {
	return taskListTool{manager: manager}
}

func NewTaskGetTool(manager *Manager) tools.Tool {
	return taskGetTool{manager: manager}
}

func NewTaskStopTool(manager *Manager) tools.Tool {
	return taskStopTool{manager: manager}
}

func NewSendMessageTool(manager *Manager) tools.Tool {
	return sendMessageTool{manager: manager}
}

type taskListTool struct{ manager *Manager }
type taskGetTool struct{ manager *Manager }
type taskStopTool struct{ manager *Manager }
type sendMessageTool struct{ manager *Manager }

func (t taskListTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "TaskList",
		Description: "List background sub Agent tasks and their current status.",
		Safety:      tools.SafetyReadOnly,
		InputSchema: objectSchema(nil),
	}
}

func (t taskListTool) Execute(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	return jsonResult("TaskList", map[string]any{"tasks": t.manager.List()})
}

func (t taskGetTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "TaskGet",
		Description: "Get details for a background sub Agent task.",
		Safety:      tools.SafetyReadOnly,
		InputSchema: objectSchema(map[string]any{"task_id": stringProp("Task id to inspect.")}, "task_id"),
	}
}

func (t taskGetTool) Execute(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := decode(input, &args); err != nil {
		return tools.Failure("TaskGet", "invalid_arguments", err.Error(), nil)
	}
	if args.TaskID == "" {
		return tools.Failure("TaskGet", "invalid_arguments", "task_id is required", nil)
	}
	snapshot, ok := t.manager.Get(args.TaskID)
	if !ok {
		return tools.Failure("TaskGet", "not_found", "task not found", map[string]any{"task_id": args.TaskID})
	}
	return jsonResult("TaskGet", snapshot)
}

func (t taskStopTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "TaskStop",
		Description: "Cancel a running background sub Agent task.",
		Safety:      tools.SafetySideEffect,
		InputSchema: objectSchema(map[string]any{"task_id": stringProp("Task id to stop.")}, "task_id"),
	}
}

func (t taskStopTool) Execute(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := decode(input, &args); err != nil {
		return tools.Failure("TaskStop", "invalid_arguments", err.Error(), nil)
	}
	if args.TaskID == "" {
		return tools.Failure("TaskStop", "invalid_arguments", "task_id is required", nil)
	}
	if !t.manager.Stop(args.TaskID) {
		return tools.Failure("TaskStop", "not_found", "task not found", map[string]any{"task_id": args.TaskID})
	}
	return jsonResult("TaskStop", map[string]any{"task_id": args.TaskID, "status": "cancellation_requested"})
}

func (t sendMessageTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "SendMessage",
		Description: "Send a follow-up message to a completed named background sub Agent.",
		Safety:      tools.SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"name":    stringProp("Named background sub Agent."),
			"message": stringProp("Follow-up message."),
		}, "name", "message"),
	}
}

func (t sendMessageTool) Execute(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	var args struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := decode(input, &args); err != nil {
		return tools.Failure("SendMessage", "invalid_arguments", err.Error(), nil)
	}
	if args.Name == "" || args.Message == "" {
		return tools.Failure("SendMessage", "invalid_arguments", "name and message are required", nil)
	}
	id, err := t.manager.SendMessage(ctx, args.Name, args.Message)
	if err != nil {
		return tools.Failure("SendMessage", "invalid_state", err.Error(), nil)
	}
	return jsonResult("SendMessage", map[string]any{"task_id": id, "status": StatusRunning})
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

func decode(input json.RawMessage, out any) error {
	if len(input) == 0 {
		return errors.New("arguments are required")
	}
	return json.Unmarshal(input, out)
}

func jsonResult(tool string, value any) tools.Result {
	data, err := json.Marshal(value)
	if err != nil {
		return tools.Failure(tool, "serialization_error", err.Error(), nil)
	}
	return tools.Success(tool, string(data), nil)
}
