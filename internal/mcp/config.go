package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

type rawConfig struct {
	MCPServers map[string]rawServer `yaml:"mcp_servers"`
}

type rawServer struct {
	Type     string            `yaml:"type"`
	Command  string            `yaml:"command"`
	Args     []string          `yaml:"args"`
	Env      map[string]string `yaml:"env"`
	URL      string            `yaml:"url"`
	Headers  map[string]string `yaml:"headers"`
	ReadOnly bool              `yaml:"read_only"`
}

func LoadConfig(root string) (Config, []LoadIssue) {
	userPath, projectPath := configPaths(root)
	user, userIssues := loadRawConfig(userPath)
	project, projectIssues := loadRawConfig(projectPath)
	issues := append([]LoadIssue{}, userIssues...)
	issues = append(issues, projectIssues...)

	merged := mergeRawServers(user.MCPServers, project.MCPServers)
	servers := make(map[string]ServerConfig)
	for name, raw := range merged {
		expandedEnv, envIssues := expandMapValues(name, raw.Env)
		expandedHeaders, headerIssues := expandMapValues(name, raw.Headers)
		issues = append(issues, envIssues...)
		issues = append(issues, headerIssues...)
		raw.Env = expandedEnv
		raw.Headers = expandedHeaders

		server, validationIssues, ok := validateServer(name, raw)
		issues = append(issues, validationIssues...)
		if ok {
			servers[name] = server
		}
	}
	return Config{Servers: servers}, issues
}

func configPaths(root string) (string, string) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(home, ".PseudoClaude", "config.yaml"), filepath.Join(root, ".PseudoClaude", "config.yaml")
}

func loadRawConfig(path string) (rawConfig, []LoadIssue) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return rawConfig{}, nil
		}
		return rawConfig{}, []LoadIssue{{Path: path, Message: "无法读取 MCP 配置层，已跳过"}}
	}
	var cfg rawConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return rawConfig{}, []LoadIssue{{Path: path, Message: "MCP 配置 YAML 格式非法，已跳过"}}
	}
	return cfg, nil
}

func mergeRawServers(user, project map[string]rawServer) map[string]rawServer {
	merged := make(map[string]rawServer, len(user)+len(project))
	for name, server := range user {
		merged[name] = server
	}
	for name, server := range project {
		merged[name] = server
	}
	return merged
}

func expandMapValues(server string, values map[string]string) (map[string]string, []LoadIssue) {
	if values == nil {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	var issues []LoadIssue
	seenMissing := map[string]bool{}
	for key, value := range values {
		expanded, missing := expandVars(value)
		out[key] = expanded
		for _, name := range missing {
			if seenMissing[name] {
				continue
			}
			seenMissing[name] = true
			issues = append(issues, LoadIssue{
				Server:  server,
				Message: fmt.Sprintf("环境变量 %s 未定义，已展开为空字符串", name),
			})
		}
	}
	return out, issues
}

func expandVars(value string) (string, []string) {
	missingSet := map[string]bool{}
	expanded := envPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := envPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		name := parts[1]
		replacement, ok := os.LookupEnv(name)
		if !ok {
			missingSet[name] = true
			return ""
		}
		return replacement
	})
	missing := make([]string, 0, len(missingSet))
	for name := range missingSet {
		missing = append(missing, name)
	}
	return expanded, missing
}

func validateServer(name string, raw rawServer) (ServerConfig, []LoadIssue, bool) {
	typ := TransportType(strings.TrimSpace(raw.Type))
	if typ == "" {
		return ServerConfig{}, []LoadIssue{{Server: name, Message: "缺少 type，已跳过该 MCP Server"}}, false
	}
	switch typ {
	case TransportStdio:
		command := strings.TrimSpace(raw.Command)
		if command == "" {
			return ServerConfig{}, []LoadIssue{{Server: name, Message: "stdio MCP Server 缺少 command，已跳过"}}, false
		}
		return ServerConfig{
			Type:     TransportStdio,
			Command:  command,
			Args:     append([]string(nil), raw.Args...),
			Env:      cloneStringMap(raw.Env),
			ReadOnly: raw.ReadOnly,
		}, nil, true
	case TransportHTTP:
		url := strings.TrimSpace(raw.URL)
		if url == "" {
			return ServerConfig{}, []LoadIssue{{Server: name, Message: "http MCP Server 缺少 url，已跳过"}}, false
		}
		return ServerConfig{
			Type:     TransportHTTP,
			URL:      url,
			Headers:  cloneStringMap(raw.Headers),
			ReadOnly: raw.ReadOnly,
		}, nil, true
	default:
		return ServerConfig{}, []LoadIssue{{Server: name, Message: fmt.Sprintf("不支持的 MCP transport type %q，已跳过", raw.Type)}}, false
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
