package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"time"

	"PseudoClaude/internal/skills"
)

type skillTool struct {
	spec    skills.ToolSpec
	timeout time.Duration
}

func NewSkillTool(spec skills.ToolSpec) Tool {
	return skillTool{spec: spec, timeout: 30 * time.Second}
}

func (s skillTool) Definition() Definition {
	schema := s.spec.InputSchema
	if schema == nil {
		schema = objectSchema(nil)
	}
	return Definition{
		Name:        s.spec.Name,
		Description: s.spec.Description,
		InputSchema: schema,
		Safety:      SafetySideEffect,
	}
}

func (s skillTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	if len(s.spec.Command) == 0 {
		return Failure(s.spec.Name, "invalid_tool", "command is empty", nil)
	}
	timeout := s.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(execCtx, s.spec.Command[0], s.spec.Command[1:]...)
	cmd.Dir = s.spec.RootDir
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out, outTruncated := truncateString(stdout.String(), env.MaxOutputBytes)
	errOut, errTruncated := truncateString(stderr.String(), env.MaxOutputBytes)
	metadata := map[string]any{
		"stderr":           errOut,
		"stdout_truncated": outTruncated,
		"stderr_truncated": errTruncated,
	}
	if execCtx.Err() != nil {
		return Failure(s.spec.Name, "timeout", execCtx.Err().Error(), metadata)
	}
	if err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		metadata["exit_code"] = exitCode
		result := Failure(s.spec.Name, "command_failed", err.Error(), metadata)
		result.Content = out
		return result
	}
	metadata["exit_code"] = 0
	return Success(s.spec.Name, out, metadata)
}
