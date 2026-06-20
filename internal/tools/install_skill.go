package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"PseudoClaude/internal/skills"
)

type InstallSkillTool struct {
	UserDir string
	Reload  func() skills.ReloadResult
}

func NewInstallSkillTool(userDir string, reload func() skills.ReloadResult) Tool {
	return InstallSkillTool{UserDir: userDir, Reload: reload}
}

func (t InstallSkillTool) Definition() Definition {
	return Definition{
		Name:        "install_skill",
		Description: "Install a local or HTTP(S) zip skill package into the user skill directory.",
		Safety:      SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"source": stringProp("Local zip path or HTTP(S) URL."),
		}, "source"),
	}
}

func (t InstallSkillTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	var args struct {
		Source string `json:"source"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return invalidArgs("install_skill", err)
	}
	if args.Source == "" {
		return invalidArgs("install_skill", errors.New("source is required"))
	}
	result, err := skills.Install(ctx, args.Source, t.UserDir)
	if err != nil {
		return Failure("install_skill", "install_failed", err.Error(), nil)
	}
	var reload skills.ReloadResult
	if t.Reload != nil {
		reload = t.Reload()
	}
	return Success("install_skill", fmt.Sprintf("installed skill package %q at %s", result.Name, result.Path), map[string]any{
		"name":            result.Name,
		"path":            result.Path,
		"reload_added":    reload.Added,
		"reload_removed":  reload.Removed,
		"reload_updated":  reload.Updated,
		"reload_warnings": len(reload.Warnings),
	})
}
