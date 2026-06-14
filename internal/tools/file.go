package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	errIsDir  = errors.New("path is a directory")
	errBinary = errors.New("content is not valid text")
)

type readFileTool struct{}
type writeFileTool struct{}
type editFileTool struct{}

func NewReadFileTool() Tool  { return readFileTool{} }
func NewWriteFileTool() Tool { return writeFileTool{} }
func NewEditFileTool() Tool  { return editFileTool{} }

func (readFileTool) Definition() Definition {
	return Definition{
		Name:        "read_file",
		Description: "Read a UTF-8 text file from the local workspace.",
		Safety:      SafetyReadOnly,
		InputSchema: objectSchema(map[string]any{
			"path": stringProp("Path to the file to read."),
		}, "path"),
	}
}

func (readFileTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	var args struct {
		Path *string `json:"path"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return invalidArgs("read_file", err)
	}
	if args.Path == nil {
		return invalidArgs("read_file", errors.New("path is required"))
	}
	path, err := resolvePath(env, *args.Path)
	if err != nil {
		return invalidArgs("read_file", err)
	}
	select {
	case <-ctx.Done():
		return timeoutResult("read_file", ctx)
	default:
	}
	content, truncated, err := readTextFile(path, env.MaxReadBytes)
	if err != nil {
		return fileError("read_file", err, path)
	}
	content, outputTruncated := truncateString(content, env.MaxOutputBytes)
	return Success("read_file", content, map[string]any{
		"path":      path,
		"truncated": truncated || outputTruncated,
	})
}

func (writeFileTool) Definition() Definition {
	return Definition{
		Name:        "write_file",
		Description: "Write complete UTF-8 text content to a local file, creating parent directories when needed.",
		Safety:      SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"path":    stringProp("Path to the file to write."),
			"content": stringProp("Complete file content to write."),
		}, "path", "content"),
	}
}

func (writeFileTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	var args struct {
		Path    *string `json:"path"`
		Content *string `json:"content"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return invalidArgs("write_file", err)
	}
	if args.Path == nil {
		return invalidArgs("write_file", errors.New("path is required"))
	}
	if args.Content == nil {
		return invalidArgs("write_file", errors.New("content is required"))
	}
	path, err := resolvePath(env, *args.Path)
	if err != nil {
		return invalidArgs("write_file", err)
	}
	if !isText([]byte(*args.Content)) {
		return Failure("write_file", "unsupported_content", "content is not valid UTF-8 text", map[string]any{"path": path})
	}
	select {
	case <-ctx.Done():
		return timeoutResult("write_file", ctx)
	default:
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Failure("write_file", "io_error", err.Error(), map[string]any{"path": path})
	}
	if err := os.WriteFile(path, []byte(*args.Content), 0o644); err != nil {
		return Failure("write_file", "io_error", err.Error(), map[string]any{"path": path})
	}
	return Success("write_file", fmt.Sprintf("wrote %d bytes", len(*args.Content)), map[string]any{
		"path":  path,
		"bytes": len(*args.Content),
	})
}

func (editFileTool) Definition() Definition {
	return Definition{
		Name:        "edit_file",
		Description: "Replace old_text with new_text only when old_text appears exactly once in a text file.",
		Safety:      SafetySideEffect,
		InputSchema: objectSchema(map[string]any{
			"path":     stringProp("Path to the file to edit."),
			"old_text": stringProp("Exact text to replace; must appear exactly once."),
			"new_text": stringProp("Replacement text."),
		}, "path", "old_text", "new_text"),
	}
}

func (editFileTool) Execute(ctx context.Context, input json.RawMessage, env Env) Result {
	var args struct {
		Path    *string `json:"path"`
		OldText *string `json:"old_text"`
		NewText *string `json:"new_text"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return invalidArgs("edit_file", err)
	}
	if args.Path == nil {
		return invalidArgs("edit_file", errors.New("path is required"))
	}
	if args.OldText == nil || *args.OldText == "" {
		return invalidArgs("edit_file", errors.New("old_text is required"))
	}
	if args.NewText == nil {
		return invalidArgs("edit_file", errors.New("new_text is required"))
	}
	path, err := resolvePath(env, *args.Path)
	if err != nil {
		return invalidArgs("edit_file", err)
	}
	select {
	case <-ctx.Done():
		return timeoutResult("edit_file", ctx)
	default:
	}
	content, truncated, err := readTextFile(path, env.MaxReadBytes)
	if err != nil {
		return fileError("edit_file", err, path)
	}
	if truncated {
		return Failure("edit_file", "output_too_large", "file exceeds MaxReadBytes; refusing to edit truncated content", map[string]any{"path": path})
	}
	count := strings.Count(content, *args.OldText)
	if count != 1 {
		return Failure("edit_file", "not_unique", fmt.Sprintf("old_text matched %d times; expected exactly 1", count), map[string]any{
			"path":        path,
			"match_count": count,
		})
	}
	next := strings.Replace(content, *args.OldText, *args.NewText, 1)
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return Failure("edit_file", "io_error", err.Error(), map[string]any{"path": path})
	}
	return Success("edit_file", "replaced 1 occurrence", map[string]any{
		"path":         path,
		"replacements": 1,
		"size_delta":   len(next) - len(content),
	})
}

func decodeArgs(input json.RawMessage, out any) error {
	if len(input) == 0 {
		return errors.New("arguments are required")
	}
	dec := json.NewDecoder(strings.NewReader(string(input)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func invalidArgs(tool string, err error) Result {
	return Failure(tool, "invalid_arguments", err.Error(), nil)
}

func timeoutResult(tool string, ctx context.Context) Result {
	return Failure(tool, "timeout", ctx.Err().Error(), nil)
}

func fileError(tool string, err error, path string) Result {
	metadata := map[string]any{"path": path}
	switch {
	case errors.Is(err, os.ErrNotExist):
		return Failure(tool, "not_found", err.Error(), metadata)
	case errors.Is(err, os.ErrPermission):
		return Failure(tool, "permission_denied", err.Error(), metadata)
	case errors.Is(err, errIsDir):
		return Failure(tool, "invalid_path", "path points to a directory", metadata)
	case errors.Is(err, errBinary):
		return Failure(tool, "unsupported_content", "file content is not valid text", metadata)
	default:
		return Failure(tool, "io_error", err.Error(), metadata)
	}
}
