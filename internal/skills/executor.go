package skills

import (
	"fmt"
)

type Runner interface {
	RunShared(label, text string) error
	RunIsolated(input IsolatedInput) error
}

type IsolatedInput struct {
	Name    string
	Text    string
	Tools   []string
	History HistoryMode
	Model   string
}

type Executor struct {
	Catalog *Catalog
	Active  *ActiveSkills
	Runner  Runner
}

func (e Executor) Execute(name, args string) error {
	if e.Catalog == nil {
		return fmt.Errorf("skill catalog is not configured")
	}
	skill, ok := e.Catalog.Get(name)
	if !ok {
		return fmt.Errorf("unknown skill %q", name)
	}
	if latest, err := ReloadSkillBody(skill); err == nil {
		skill = latest
	}
	rendered := RenderInvocation(skill, args)
	if e.Active != nil {
		e.Active.Activate(skill.Meta.Name, rendered)
	}
	if e.Runner == nil {
		return nil
	}
	switch skill.Meta.Mode {
	case ModeIsolated:
		return e.Runner.RunIsolated(IsolatedInput{
			Name:    skill.Meta.Name,
			Text:    rendered,
			Tools:   skill.Meta.Tools,
			History: skill.Meta.History,
			Model:   skill.Meta.Model,
		})
	default:
		return e.Runner.RunShared("/"+skill.Meta.Name, rendered)
	}
}
