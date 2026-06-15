package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

type runCommandTool struct{}

func NewRunCommandTool() Tool { return runCommandTool{} }

func (runCommandTool) Definition() Definition {
	return Definition{
		Name:        "run_command",
		Description: "Run a local command in the current workspace and return stdout, stderr, and exit status. Prefer read_file, find_files, and search_code for reading, locating, or searching files; use commands for build, test, validation, or shell-only operations.",
		Safety:      SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"command": stringProp("Executable command name or path."),
			"args": map[string]any{
				"type":        "array",
				"description": "Optional command arguments.",
				"items":       map[string]any{"type": "string"},
			},
		}, "command"),
	}
}

func (runCommandTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	var args struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return invalidArgs("run_command", err)
	}
	if args.Command == "" {
		return invalidArgs("run_command", errors.New("command is required"))
	}

	cmd := exec.CommandContext(ctx, args.Command, args.Args...)
	cmd.Dir = env.CWD
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	out, outTruncated := truncateString(stdout.String(), env.MaxOutputBytes)
	errOut, errTruncated := truncateString(stderr.String(), env.MaxOutputBytes)
	content := formatCommandContent(out, errOut)
	metadata := map[string]any{
		"stdout":           out,
		"stderr":           errOut,
		"stdout_truncated": outTruncated,
		"stderr_truncated": errTruncated,
	}

	if ctx.Err() != nil {
		metadata["exit_code"] = -1
		return Failure("run_command", "timeout", ctx.Err().Error(), metadata)
	}
	if err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		metadata["exit_code"] = exitCode
		result := Failure("run_command", "command_failed", fmt.Sprintf("command exited with status %d", exitCode), metadata)
		result.Content = content
		return result
	}
	metadata["exit_code"] = 0
	return Success("run_command", content, metadata)
}

func formatCommandContent(stdout, stderr string) string {
	if stderr == "" {
		return stdout
	}
	if stdout == "" {
		return stderr
	}
	return stdout + "\n" + stderr
}
