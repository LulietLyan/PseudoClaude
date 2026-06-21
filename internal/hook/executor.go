package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

type Executor struct {
	Client *http.Client
	Logf   func(format string, args ...any)
}

type ExecutionResult struct {
	Blocked bool
	Reason  string
	Prompt  string
	Err     error
}

func (x Executor) Run(ctx context.Context, rule Rule, payload Payload, blocking bool) ExecutionResult {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := rule.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	switch rule.Action.Type {
	case ActionPrompt:
		if rule.Action.Prompt == nil {
			return ExecutionResult{Err: fmt.Errorf("prompt action missing config")}
		}
		return ExecutionResult{Prompt: rule.Action.Prompt.Text}
	case ActionSubagent:
		if rule.Action.Subagent == nil {
			return ExecutionResult{Err: fmt.Errorf("subagent action missing config")}
		}
		logf(x.Logf, "[hook subagent] not yet implemented, skipped: %s", rule.Action.Subagent.AgentName)
		return ExecutionResult{}
	case ActionShell:
		return x.runShell(ctx, rule, payload, blocking)
	case ActionHTTP:
		return x.runHTTP(ctx, rule, payload, blocking)
	default:
		return ExecutionResult{Err: fmt.Errorf("unknown action type %q", rule.Action.Type)}
	}
}

func (x Executor) runShell(ctx context.Context, rule Rule, payload Payload, blocking bool) ExecutionResult {
	if rule.Action.Shell == nil {
		return ExecutionResult{Err: fmt.Errorf("shell action missing config")}
	}
	data, err := payload.JSON()
	if err != nil {
		return ExecutionResult{Err: err}
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", rule.Action.Shell.Command)
	cmd.Stdin = bytes.NewReader(append(data, '\n'))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		return ExecutionResult{Err: ctx.Err()}
	}
	if err == nil {
		return ExecutionResult{}
	}
	if exitErr, ok := err.(*exec.ExitError); ok && blocking && exitErr.ExitCode() == 2 {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = strings.TrimSpace(stdout.String())
		}
		if reason == "" {
			reason = "blocked by hook"
		}
		return ExecutionResult{Blocked: true, Reason: reason}
	}
	combined := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
	if combined == "" {
		combined = err.Error()
	}
	return ExecutionResult{Err: fmt.Errorf("shell hook failed: %s", combined)}
}

func (x Executor) runHTTP(ctx context.Context, rule Rule, payload Payload, blocking bool) ExecutionResult {
	if rule.Action.HTTP == nil {
		return ExecutionResult{Err: fmt.Errorf("http action missing config")}
	}
	body, err := renderHTTPBody(rule.Action.HTTP.Body, payload)
	if err != nil {
		return ExecutionResult{Err: err}
	}
	method := strings.TrimSpace(rule.Action.HTTP.Method)
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, rule.Action.HTTP.URL, bytes.NewReader(body))
	if err != nil {
		return ExecutionResult{Err: err}
	}
	if rule.Action.HTTP.Body == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range rule.Action.HTTP.Headers {
		req.Header.Set(key, value)
	}
	client := x.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return ExecutionResult{Err: err}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ExecutionResult{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExecutionResult{Err: fmt.Errorf("http hook returned %s", resp.Status)}
	}
	if !blocking {
		return ExecutionResult{}
	}
	var decision struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(respBody, &decision); err != nil {
		return ExecutionResult{Err: err}
	}
	if strings.EqualFold(decision.Decision, "block") {
		reason := strings.TrimSpace(decision.Reason)
		if reason == "" {
			reason = "blocked by hook"
		}
		return ExecutionResult{Blocked: true, Reason: reason}
	}
	return ExecutionResult{}
}

func renderHTTPBody(tmpl string, payload Payload) ([]byte, error) {
	if strings.TrimSpace(tmpl) == "" {
		return payload.JSON()
	}
	parsed, err := template.New("hook-body").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := parsed.Execute(&b, map[string]any(payload)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func summarizeResult(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		return value[:497] + "..."
	}
	return value
}

func timeoutOrDefault(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return defaultTimeout
}
