package command

import "fmt"

func RegisterSkillCommands(reg *Registry, summaries []SkillSummary) []error {
	var errs []error
	for _, summary := range summaries {
		summary := summary
		cmd := Command{
			Name:        "/" + summary.Name,
			Description: summary.Description,
			Usage:       "/" + summary.Name + " [arguments]",
			Kind:        KindSkill,
			ArgHint:     "[arguments]",
			Skill:       true,
			Handler: func(ctx Context, ctl Controller) error {
				return ctl.RunSkill(summary.Name, ctx.Args)
			},
		}
		if err := reg.Register(cmd); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", summary.Name, err))
		}
	}
	return errs
}

func RemoveSkillCommands(reg *Registry) {
	reg.RemoveWhere(func(cmd Command) bool { return cmd.Skill || cmd.Kind == KindSkill })
}
