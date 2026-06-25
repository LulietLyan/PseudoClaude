package tools

import (
	"context"
	"encoding/json"

	"PseudoClaude/internal/team"
	"PseudoClaude/internal/team/teamtask"
	base "PseudoClaude/internal/tools"
)

type TaskCreateTool struct{ Manager *team.Manager }
type TaskUpdateTool struct{ Manager *team.Manager }
type TaskListTool struct{ Manager *team.Manager }
type TaskGetTool struct{ Manager *team.Manager }

func NewTaskCreateTool(manager *team.Manager) base.Tool { return TaskCreateTool{Manager: manager} }
func NewTaskUpdateTool(manager *team.Manager) base.Tool { return TaskUpdateTool{Manager: manager} }
func NewTaskListTool(manager *team.Manager) base.Tool   { return TaskListTool{Manager: manager} }
func NewTaskGetTool(manager *team.Manager) base.Tool    { return TaskGetTool{Manager: manager} }

func (t TaskCreateTool) Definition() base.Definition {
	return base.Definition{Name: "TaskCreate", Description: "Create a shared team task.", Safety: base.SafetySideEffect, InputSchema: objectSchema(map[string]any{
		"team_name":   stringProp("Optional team name. Defaults to current team context."),
		"title":       stringProp("Task title."),
		"description": stringProp("Task description."),
		"assignee":    stringProp("Assigned member name."),
	}, "title")}
}

func (t TaskCreateTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	var args struct {
		TeamName    string `json:"team_name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Assignee    string `json:"assignee"`
	}
	if err := decode(input, &args); err != nil {
		return base.Failure("TaskCreate", "invalid_arguments", err.Error(), nil)
	}
	name := teamName(args.TeamName, env)
	if name == "" {
		return base.Failure("TaskCreate", "invalid_arguments", "team_name is required outside team context", nil)
	}
	store, err := t.Manager.TaskStore(name)
	if err != nil {
		return teamFailure("TaskCreate", err)
	}
	task, err := store.Create(ctx, teamtask.CreateInput{Title: args.Title, Description: args.Description, Assignee: args.Assignee})
	if err != nil {
		return teamFailure("TaskCreate", err)
	}
	return jsonResult("TaskCreate", task)
}

func (t TaskUpdateTool) Definition() base.Definition {
	return base.Definition{Name: "TaskUpdate", Description: "Update a shared team task.", Safety: base.SafetySideEffect, InputSchema: objectSchema(map[string]any{
		"team_name":         stringProp("Optional team name. Defaults to current team context."),
		"task_id":           stringProp("Task id."),
		"title":             stringProp("Optional new title."),
		"description":       stringProp("Optional new description."),
		"assignee":          stringProp("Optional new assignee."),
		"status":            stringProp("Optional status: todo, in_progress, done, blocked."),
		"add_blocked_by":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"remove_blocked_by": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"add_blocks":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"remove_blocks":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	}, "task_id")}
}

func (t TaskUpdateTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	var args struct {
		TeamName        string   `json:"team_name"`
		TaskID          string   `json:"task_id"`
		Title           *string  `json:"title"`
		Description     *string  `json:"description"`
		Assignee        *string  `json:"assignee"`
		Status          *string  `json:"status"`
		AddBlockedBy    []string `json:"add_blocked_by"`
		RemoveBlockedBy []string `json:"remove_blocked_by"`
		AddBlocks       []string `json:"add_blocks"`
		RemoveBlocks    []string `json:"remove_blocks"`
	}
	if err := decode(input, &args); err != nil {
		return base.Failure("TaskUpdate", "invalid_arguments", err.Error(), nil)
	}
	name := teamName(args.TeamName, env)
	if name == "" {
		return base.Failure("TaskUpdate", "invalid_arguments", "team_name is required outside team context", nil)
	}
	store, err := t.Manager.TaskStore(name)
	if err != nil {
		return teamFailure("TaskUpdate", err)
	}
	var status *teamtask.Status
	if args.Status != nil {
		value := teamtask.Status(*args.Status)
		status = &value
	}
	task, err := store.Update(ctx, args.TaskID, teamtask.Patch{
		Title:           args.Title,
		Description:     args.Description,
		Assignee:        args.Assignee,
		Status:          status,
		AddBlockedBy:    args.AddBlockedBy,
		RemoveBlockedBy: args.RemoveBlockedBy,
		AddBlocks:       args.AddBlocks,
		RemoveBlocks:    args.RemoveBlocks,
	})
	if err != nil {
		return teamFailure("TaskUpdate", err)
	}
	return jsonResult("TaskUpdate", task)
}

func (t TaskListTool) Definition() base.Definition {
	return base.Definition{Name: "TaskList", Description: "List shared team tasks.", Safety: base.SafetyReadOnly, InputSchema: objectSchema(map[string]any{
		"team_name": stringProp("Optional team name. Defaults to current team context."),
		"status":    stringProp("Optional status filter."),
	})}
}

func (t TaskListTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	var args struct {
		TeamName string `json:"team_name"`
		Status   string `json:"status"`
	}
	if err := decode(input, &args); err != nil {
		return base.Failure("TaskList", "invalid_arguments", err.Error(), nil)
	}
	name := teamName(args.TeamName, env)
	if name == "" {
		return base.Failure("TaskList", "invalid_arguments", "team_name is required outside team context", nil)
	}
	store, err := t.Manager.TaskStore(name)
	if err != nil {
		return teamFailure("TaskList", err)
	}
	tasks, err := store.List(teamtask.ListFilter{Status: teamtask.Status(args.Status)})
	if err != nil {
		return teamFailure("TaskList", err)
	}
	return jsonResult("TaskList", map[string]any{"tasks": tasks})
}

func (t TaskGetTool) Definition() base.Definition {
	return base.Definition{Name: "TaskGet", Description: "Get a shared team task.", Safety: base.SafetyReadOnly, InputSchema: objectSchema(map[string]any{
		"team_name": stringProp("Optional team name. Defaults to current team context."),
		"task_id":   stringProp("Task id."),
	}, "task_id")}
}

func (t TaskGetTool) Execute(ctx context.Context, input json.RawMessage, env base.Env) base.Result {
	var args struct {
		TeamName string `json:"team_name"`
		TaskID   string `json:"task_id"`
	}
	if err := decode(input, &args); err != nil {
		return base.Failure("TaskGet", "invalid_arguments", err.Error(), nil)
	}
	name := teamName(args.TeamName, env)
	if name == "" {
		return base.Failure("TaskGet", "invalid_arguments", "team_name is required outside team context", nil)
	}
	store, err := t.Manager.TaskStore(name)
	if err != nil {
		return teamFailure("TaskGet", err)
	}
	task, ok, err := store.Get(args.TaskID)
	if err != nil {
		return teamFailure("TaskGet", err)
	}
	if !ok {
		return base.Failure("TaskGet", "not_found", "task not found", map[string]any{"task_id": args.TaskID})
	}
	return jsonResult("TaskGet", task)
}
