package command

import "strings"

func Builtins() []Command {
	return []Command{
		{
			Name:        "/help",
			Aliases:     []string{"/h", "/?"},
			Description: "Show available slash commands.",
			Usage:       "/help",
			Kind:        KindLocal,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.Show(MessageHelp, "Help is unavailable.")
				return nil
			},
		},
		{
			Name:        "/compact",
			Aliases:     []string{"/compress"},
			Description: "Compact the current conversation context.",
			Usage:       "/compact",
			Kind:        KindUI,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.TriggerCompact()
				return nil
			},
		},
		{
			Name:        "/clear",
			Aliases:     []string{"/cls"},
			Description: "Clear the visible conversation area.",
			Usage:       "/clear",
			Kind:        KindUI,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.ClearScreen()
				ctl.ClearActiveSkills()
				ctl.Show(MessageInfo, "Screen cleared. Conversation context is preserved.")
				return nil
			},
		},
		{
			Name:        "/plan",
			Aliases:     []string{"/p"},
			Description: "Switch to plan work mode.",
			Usage:       "/plan",
			Kind:        KindUI,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.SetWorkMode(WorkModePlan)
				ctl.Show(MessageInfo, "Work mode: PLAN.")
				ctl.RefreshStatus()
				return nil
			},
		},
		{
			Name:        "/do",
			Aliases:     []string{"/default"},
			Description: "Switch to default work mode.",
			Usage:       "/do",
			Kind:        KindUI,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.SetWorkMode(WorkModeDefault)
				ctl.Show(MessageInfo, "Work mode: DEFAULT.")
				ctl.RefreshStatus()
				return nil
			},
		},
		{
			Name:        "/session",
			Aliases:     []string{"/sess"},
			Description: "Show current session information.",
			Usage:       "/session",
			Kind:        KindLocal,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.Show(MessageInfo, FormatSession(ctl.Session()))
				return nil
			},
		},
		{
			Name:        "/memory",
			Aliases:     []string{"/mem"},
			Description: "Show long-term memory summary.",
			Usage:       "/memory",
			Kind:        KindLocal,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.Show(MessageInfo, FormatMemory(ctl.MemorySummary()))
				return nil
			},
		},
		{
			Name:        "/permission",
			Aliases:     []string{"/perm"},
			Description: "Show current permission mode.",
			Usage:       "/permission",
			Kind:        KindLocal,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.Show(MessageInfo, "Permission mode: "+ctl.PermissionMode())
				return nil
			},
		},
		{
			Name:        "/skill",
			Aliases:     []string{"/skills"},
			Description: "List available skills.",
			Usage:       "/skill [reload]",
			Kind:        KindLocal,
			ArgHint:     "[reload]",
			Handler: func(ctx Context, ctl Controller) error {
				if strings.EqualFold(strings.TrimSpace(ctx.Args), "reload") {
					ctl.ReloadSkills()
				}
				ctl.Show(MessageInfo, FormatSkills(ctl.ListSkills()))
				return nil
			},
		},
		{
			Name:        "/hooks",
			Description: "List loaded lifecycle hooks.",
			Usage:       "/hooks",
			Kind:        KindLocal,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.Show(MessageInfo, FormatHooks(ctl.ListHooks(), ctl.HookSources()))
				return nil
			},
		},
		{
			Name:        "/agents",
			Aliases:     []string{"/agent"},
			Description: "List and inspect sub Agent roles.",
			Usage:       "/agents [reload|name]",
			Kind:        KindLocal,
			ArgHint:     "[reload|name]",
			Handler: func(ctx Context, ctl Controller) error {
				arg := strings.TrimSpace(ctx.Args)
				if strings.EqualFold(arg, "reload") {
					ctl.ReloadAgents()
					ctl.Show(MessageInfo, FormatAgents(ctl.ListAgents()))
					return nil
				}
				if arg != "" {
					detail, ok := ctl.DescribeAgent(arg)
					if !ok {
						ctl.Show(MessageError, "Sub agent not found: "+arg)
						return nil
					}
					ctl.Show(MessageInfo, FormatAgentDetail(detail))
					return nil
				}
				ctl.Show(MessageInfo, FormatAgents(ctl.ListAgents()))
				return nil
			},
		},
		{
			Name:        "/worktree",
			Aliases:     []string{"/wt"},
			Description: "Manage isolated Git worktrees.",
			Usage:       "/worktree [create|list|enter|exit|remove]",
			Kind:        KindUI,
			ArgHint:     "[create|list|enter|exit|remove]",
			Handler:     handleWorktreeCommand,
		},
		{
			Name:        "/team",
			Description: "Manage persistent collaboration teams.",
			Usage:       "/team [list|info|delete|kill]",
			Kind:        KindUI,
			ArgHint:     "[list|info|delete|kill]",
			Handler:     handleTeamCommand,
		},
		{
			Name:        "/status",
			Aliases:     []string{"/st"},
			Description: "Show runtime status.",
			Usage:       "/status",
			Kind:        KindLocal,
			Handler: func(ctx Context, ctl Controller) error {
				ctl.Show(MessageInfo, FormatStatus(ctl.Status()))
				return nil
			},
		},
	}
}

func NewBuiltinRegistry() *Registry {
	commands := Builtins()
	reg := MustNewRegistry(commands)
	for i := range reg.entries {
		if reg.entries[i].Name == "/help" {
			reg.entries[i].Handler = func(ctx Context, ctl Controller) error {
				ctl.Show(MessageHelp, FormatHelp(reg.Visible()))
				return nil
			}
			break
		}
	}
	return reg
}
