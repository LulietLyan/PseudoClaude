package mcp

import (
	"context"
	"io"
	"time"

	"PseudoClaude/internal/tools"
)

const (
	defaultConnectTimeout = 30 * time.Second
	defaultCallTimeout    = 30 * time.Second
	defaultCloseTimeout   = 5 * time.Second
)

type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportHTTP  TransportType = "http"
)

type Config struct {
	Servers map[string]ServerConfig
}

type ServerConfig struct {
	Type     TransportType
	Command  string
	Args     []string
	Env      map[string]string
	URL      string
	Headers  map[string]string
	ReadOnly bool
}

type LoadIssue struct {
	Path    string
	Server  string
	Message string
}

type Issue struct {
	Server  string
	Tool    string
	Stage   string
	Message string
}

type ClientInfo struct {
	Name    string
	Version string
}

type RemoteTool struct {
	Name        string
	Description string
	InputSchema map[string]any
	ReadOnly    bool
}

type CallResult struct {
	TextBlocks     []string
	IsError        bool
	NonTextDropped int
	Metadata       map[string]any
}

type ClientSession interface {
	ListTools(ctx context.Context) ([]RemoteTool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (CallResult, error)
	Close() error
}

type Dialer interface {
	Dial(ctx context.Context, name string, cfg ServerConfig, info ClientInfo) (ClientSession, error)
}

type ManagerOptions struct {
	ConnectTimeout time.Duration
	CallTimeout    time.Duration
	CloseTimeout   time.Duration
	ClientInfo     ClientInfo
	Dialer         Dialer
	Err            io.Writer
	OnIssue        func(Issue)
}

type Manager struct {
	closeTimeout time.Duration

	sessions []*serverSession
	tools    []tools.Tool
	issues   []Issue
	stats    Stats
}

type Stats struct {
	Configured int
	Connected  int
	Discovered int
	Adapted    int
	Registered int
	ReadOnly   int
	SideEffect int
}

type serverSession struct {
	name    string
	session ClientSession
}
