package command

type DispatchResult struct {
	Handled bool
	Kind    Kind
	Err     error
}

func Dispatch(reg *Registry, input string, ctl Controller) DispatchResult {
	parsed := Parse(input)
	if parsed.Empty {
		return DispatchResult{Handled: true}
	}
	if !parsed.IsSlash {
		return DispatchResult{Handled: false}
	}
	cmd, ok := reg.Lookup(parsed.Token)
	if !ok {
		ctl.Show(MessageHelp, "Unknown command.\n"+FormatHelpHint())
		return DispatchResult{Handled: true}
	}
	result := DispatchResult{Handled: true, Kind: cmd.Kind}
	if parsed.Args != "" && cmd.ArgHint == "" {
		ctl.Show(MessageHelp, "Invalid usage for "+cmd.Name+".\n"+FormatHelpHint())
		return result
	}
	if (cmd.Kind == KindUI || cmd.Kind == KindPrompt) && !ctl.IsIdle() {
		ctl.Show(MessageHelp, "Please wait for the current task to finish before running this command.\n"+FormatHelpHint())
		return result
	}
	ctx := Context{Input: parsed.Input, Name: cmd.Name, Args: parsed.Args, Command: cmd}
	result.Err = cmd.Handler(ctx, ctl)
	return result
}
