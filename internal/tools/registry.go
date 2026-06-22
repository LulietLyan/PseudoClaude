package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Registry struct {
	tools map[string]Tool
}

func NewRegistry(toolList ...Tool) (*Registry, error) {
	r := &Registry{tools: make(map[string]Tool)}
	for _, tool := range toolList {
		if err := r.Register(tool); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func DefaultRegistry() (*Registry, error) {
	return NewRegistry(
		NewReadFileTool(),
		NewWriteFileTool(),
		NewEditFileTool(),
		NewRunCommandTool(),
		NewFindFilesTool(),
		NewSearchCodeTool(),
	)
}

func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return errors.New("tool is nil")
	}
	def := tool.Definition()
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return errors.New("tool name is required")
	}
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = tool
	return nil
}

func (r *Registry) RegisterOrReplace(tool Tool) error {
	if tool == nil {
		return errors.New("tool is nil")
	}
	def := tool.Definition()
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return errors.New("tool name is required")
	}
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	r.tools[name] = tool
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) Definitions() []Definition {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		defs = append(defs, r.tools[name].Definition())
	}
	return defs
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) DefinitionsFiltered(allowed []string) []Definition {
	if r == nil {
		return nil
	}
	if len(allowed) == 0 {
		return r.Definitions()
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name != "" {
			allowedSet[name] = true
		}
	}
	defs := r.Definitions()
	out := make([]Definition, 0, len(defs))
	for _, def := range defs {
		if allowedSet[def.Name] || def.System {
			out = append(out, def)
		}
	}
	return out
}

func (r *Registry) DefinitionsBySafety(allowed ...Safety) []Definition {
	if r == nil || len(allowed) == 0 {
		return nil
	}
	allowedSet := make(map[Safety]bool, len(allowed))
	for _, safety := range allowed {
		allowedSet[safety] = true
	}
	defs := r.Definitions()
	out := make([]Definition, 0, len(defs))
	for _, def := range defs {
		if allowedSet[def.Safety] {
			out = append(out, def)
		}
	}
	return out
}

func (r *Registry) Safety(name string) (Safety, bool) {
	tool, ok := r.Get(name)
	if !ok {
		return "", false
	}
	return tool.Definition().Safety, true
}

func (r *Registry) IsSystem(name string) bool {
	tool, ok := r.Get(name)
	if !ok {
		return false
	}
	return tool.Definition().System
}

func (r *Registry) IsKnown(name string) bool {
	_, ok := r.Get(name)
	return ok
}

func (r *Registry) Execute(ctx context.Context, call Call, env Env) Result {
	tool, ok := r.Get(call.Name)
	if !ok {
		return Failure(call.Name, "unknown_tool", fmt.Sprintf("unknown tool %q", call.Name), map[string]any{"call_id": call.ID})
	}
	if !json.Valid(call.Arguments) {
		return Failure(call.Name, "invalid_arguments", "arguments must be valid JSON", map[string]any{"call_id": call.ID})
	}
	env = normalizeEnv(env)
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := env.Timeout
	if def := tool.Definition(); def.Timeout > 0 {
		timeout = def.Timeout
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan Result, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- Failure(call.Name, "internal_error", fmt.Sprintf("tool panicked: %v", recovered), map[string]any{"call_id": call.ID})
			}
		}()
		done <- tool.Execute(execCtx, call.Arguments, env)
	}()

	select {
	case <-execCtx.Done():
		return Failure(call.Name, "timeout", execCtx.Err().Error(), map[string]any{"call_id": call.ID})
	case result := <-done:
		if result.Tool == "" {
			result.Tool = call.Name
		}
		if result.Metadata == nil {
			result.Metadata = map[string]any{}
		}
		result.Metadata["call_id"] = call.ID
		return result
	}
}
