package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"PseudoClaude/internal/tools"
)

type remoteTool struct {
	fullName    string
	serverName  string
	remoteName  string
	definition  tools.Definition
	session     ClientSession
	callTimeout time.Duration
	nonTextOnce sync.Once
}

func AdaptTool(serverName string, remote RemoteTool, session ClientSession, callTimeout time.Duration) (tools.Tool, *Issue) {
	fullName := FullToolName(serverName, remote.Name)
	if !ValidToolName(fullName) {
		return nil, &Issue{
			Server:  serverName,
			Tool:    remote.Name,
			Stage:   "adapt_tool",
			Message: fmt.Sprintf("MCP 工具名 %q 包含 provider 不支持的字符，已跳过", fullName),
		}
	}
	description := strings.TrimSpace(remote.Description)
	if description == "" {
		description = fmt.Sprintf("Tool %q from MCP server %q.", remote.Name, serverName)
	}
	schema := remote.InputSchema
	if len(schema) == 0 {
		schema = map[string]any{"type": "object"}
	}
	safety := tools.SafetySideEffect
	if remote.ReadOnly {
		safety = tools.SafetyReadOnly
	}
	if callTimeout <= 0 {
		callTimeout = defaultCallTimeout
	}
	return &remoteTool{
		fullName:    fullName,
		serverName:  serverName,
		remoteName:  remote.Name,
		session:     session,
		callTimeout: callTimeout,
		definition: tools.Definition{
			Name:        fullName,
			Description: description,
			InputSchema: schema,
			Safety:      safety,
		},
	}, nil
}

func (t *remoteTool) Definition() tools.Definition {
	return t.definition
}

func (t *remoteTool) Execute(ctx context.Context, input json.RawMessage, env tools.Env) tools.Result {
	args, err := decodeArguments(input)
	if err != nil {
		return tools.Failure(t.fullName, "invalid_arguments", err.Error(), t.metadata(0))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, t.callTimeout)
	defer cancel()

	result, err := t.session.CallTool(callCtx, t.remoteName, args)
	if err != nil {
		message := err.Error()
		if errors.Is(callCtx.Err(), context.Canceled) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			message = callCtx.Err().Error()
		}
		return tools.Failure(t.fullName, "mcp_call_error", message, t.metadata(0))
	}
	content := strings.Join(result.TextBlocks, "\n")
	metadata := t.metadata(result.NonTextDropped)
	for key, value := range result.Metadata {
		metadata[key] = value
	}
	if result.IsError {
		if strings.TrimSpace(content) == "" {
			content = "MCP tool returned an error"
		}
		return tools.Failure(t.fullName, "mcp_tool_error", content, metadata)
	}
	return tools.Success(t.fullName, content, metadata)
}

func (t *remoteTool) metadata(nonTextDropped int) map[string]any {
	return map[string]any{
		"server":           t.serverName,
		"remote_tool":      t.remoteName,
		"non_text_dropped": nonTextDropped,
	}
}

func decodeArguments(input json.RawMessage) (map[string]any, error) {
	if len(strings.TrimSpace(string(input))) == 0 || strings.TrimSpace(string(input)) == "null" {
		return map[string]any{}, nil
	}
	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("arguments must be a JSON object")
	}
	return object, nil
}
