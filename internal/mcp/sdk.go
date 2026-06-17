package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type SDKDialer struct{}

type sdkSession struct {
	serverName string
	session    *sdkmcp.ClientSession
}

func (SDKDialer) Dial(ctx context.Context, name string, cfg ServerConfig, info ClientInfo) (ClientSession, error) {
	impl := &sdkmcp.Implementation{Name: info.Name, Version: info.Version}
	client := sdkmcp.NewClient(impl, nil)
	var transport sdkmcp.Transport
	switch cfg.Type {
	case TransportStdio:
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
		cmd.Env = mergeEnv(os.Environ(), cfg.Env)
		cmd.Stderr = &bytes.Buffer{}
		transport = &sdkmcp.CommandTransport{Command: cmd}
	case TransportHTTP:
		httpClient := &http.Client{
			Transport: headerRoundTripper{
				base:    http.DefaultTransport,
				headers: cfg.Headers,
			},
		}
		transport = &sdkmcp.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpClient,
		}
	default:
		return nil, fmt.Errorf("unsupported MCP transport type %q", cfg.Type)
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	return &sdkSession{serverName: name, session: session}, nil
}

func (s *sdkSession) ListTools(ctx context.Context) ([]RemoteTool, error) {
	var out []RemoteTool
	var cursor string
	for {
		params := &sdkmcp.ListToolsParams{}
		if cursor != "" {
			params.Cursor = cursor
		}
		result, err := s.session.ListTools(ctx, params)
		if err != nil {
			return nil, err
		}
		for _, tool := range result.Tools {
			if tool == nil {
				continue
			}
			out = append(out, RemoteTool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: schemaMap(tool.InputSchema),
				ReadOnly:    tool.Annotations != nil && tool.Annotations.ReadOnlyHint,
			})
		}
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return out, nil
}

func (s *sdkSession) CallTool(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
	result, err := s.session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return CallResult{}, err
	}
	out := CallResult{
		IsError:  result.IsError,
		Metadata: map[string]any(result.Meta),
	}
	for _, content := range result.Content {
		switch c := content.(type) {
		case *sdkmcp.TextContent:
			out.TextBlocks = append(out.TextBlocks, c.Text)
		default:
			out.NonTextDropped++
		}
	}
	return out, nil
}

func (s *sdkSession) Close() error {
	return s.session.Close()
}

func schemaMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	if m, ok := schema.(map[string]any); ok {
		return cloneAnyMap(m)
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string(nil), base...)
	}
	index := make(map[string]int, len(base))
	out := append([]string(nil), base...)
	for i, pair := range out {
		for j, r := range pair {
			if r == '=' {
				index[pair[:j]] = i
				break
			}
		}
	}
	for key, value := range overrides {
		pair := key + "=" + value
		if i, ok := index[key]; ok {
			out[i] = pair
		} else {
			out = append(out, pair)
		}
	}
	return out
}
