package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"PseudoClaude/internal/skills"
)

type LoadSkillTool struct {
	Catalog  *skills.Catalog
	Active   *skills.ActiveSkills
	Registry *Registry
}

func NewLoadSkillTool(catalog *skills.Catalog, active *skills.ActiveSkills, registry *Registry) Tool {
	return LoadSkillTool{Catalog: catalog, Active: active, Registry: registry}
}

func (t LoadSkillTool) Definition() Definition {
	return Definition{
		Name:        "load_skill",
		Description: "Load a skill's full SOP into the active environment context.",
		Safety:      SafetyReadOnly,
		System:      true,
		InputSchema: objectSchema(map[string]any{
			"name": stringProp("Skill name to load."),
		}, "name"),
	}
}

func (t LoadSkillTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	var args struct {
		Name string `json:"name"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return invalidArgs("load_skill", err)
	}
	if args.Name == "" {
		return invalidArgs("load_skill", errors.New("name is required"))
	}
	if t.Catalog == nil {
		return Failure("load_skill", "not_configured", "skill catalog is not configured", nil)
	}
	skill, ok := t.Catalog.Get(args.Name)
	if !ok {
		return Failure("load_skill", "unknown_skill", fmt.Sprintf("unknown skill %q", args.Name), nil)
	}
	if latest, err := skills.ReloadSkillBody(skill); err == nil {
		skill = latest
	}
	rendered := skills.RenderInvocation(skill, "")
	if t.Active != nil {
		t.Active.Activate(skill.Meta.Name, rendered)
	}
	registered := 0
	if t.Registry != nil {
		for _, spec := range skill.Tools {
			if err := t.Registry.RegisterOrReplace(NewSkillTool(spec)); err == nil {
				registered++
			}
		}
	}
	return Success("load_skill", fmt.Sprintf("loaded skill %q (%d specialized tools registered)", skill.Meta.Name, registered), map[string]any{
		"skill":            skill.Meta.Name,
		"registered_tools": registered,
	})
}
