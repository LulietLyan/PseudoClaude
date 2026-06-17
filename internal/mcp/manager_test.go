package mcp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestManager(t *testing.T) {
	dialer := &stubDialer{
		sessions: map[string]*stubSession{
			"good": {listTools: []RemoteTool{{Name: "z"}, {Name: "a", ReadOnly: true}}},
		},
		errs: map[string]error{"bad": errors.New("boom")},
	}
	manager := NewManager(context.Background(), Config{Servers: map[string]ServerConfig{
		"bad":  {Type: TransportStdio, Command: "bad"},
		"good": {Type: TransportStdio, Command: "good", ReadOnly: true},
	}}, ManagerOptions{Dialer: dialer, ConnectTimeout: time.Second, CallTimeout: time.Second})

	tools := manager.Tools()
	if len(tools) != 2 {
		t.Fatalf("tools = %d, issues = %+v", len(tools), manager.Issues())
	}
	if tools[0].Definition().Name != "mcp__good__a" || tools[1].Definition().Name != "mcp__good__z" {
		t.Fatalf("tools not sorted: %q %q", tools[0].Definition().Name, tools[1].Definition().Name)
	}
	if tools[0].Definition().Safety != "read_only" || tools[1].Definition().Safety != "read_only" {
		t.Fatalf("read-only override not applied: %s %s", tools[0].Definition().Safety, tools[1].Definition().Safety)
	}
	stats := manager.Stats()
	if stats.Configured != 2 || stats.Connected != 1 || stats.Discovered != 2 || stats.Adapted != 2 || stats.ReadOnly != 2 || stats.SideEffect != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if len(manager.Issues()) != 1 || manager.Issues()[0].Server != "bad" {
		t.Fatalf("issues = %+v", manager.Issues())
	}
	if dialer.calls["good"] != 1 || dialer.calls["bad"] != 1 {
		t.Fatalf("calls = %+v", dialer.calls)
	}
}

func TestManagerListToolsFailureClosesSession(t *testing.T) {
	session := &stubSession{listErr: errors.New("list failed")}
	manager := NewManager(context.Background(), Config{Servers: map[string]ServerConfig{
		"s": {Type: TransportStdio, Command: "s"},
	}}, ManagerOptions{Dialer: &stubDialer{sessions: map[string]*stubSession{"s": session}}, ConnectTimeout: time.Second})
	if len(manager.Tools()) != 0 {
		t.Fatal("expected no tools")
	}
	if !session.closed {
		t.Fatal("session should be closed after ListTools failure")
	}
	if len(manager.Issues()) != 1 || manager.Issues()[0].Stage != "list_tools" {
		t.Fatalf("issues = %+v", manager.Issues())
	}
}

func TestManagerToolsReturnsCopy(t *testing.T) {
	manager := NewManager(context.Background(), Config{Servers: map[string]ServerConfig{
		"s": {Type: TransportStdio, Command: "s"},
	}}, ManagerOptions{Dialer: &stubDialer{sessions: map[string]*stubSession{"s": {listTools: []RemoteTool{{Name: "a"}}}}}, ConnectTimeout: time.Second})
	tools := manager.Tools()
	tools[0] = nil
	if manager.Tools()[0] == nil {
		t.Fatal("manager tools mutated through returned slice")
	}
}

func TestManagerClose(t *testing.T) {
	session := &stubSession{listTools: []RemoteTool{{Name: "a"}}}
	manager := NewManager(context.Background(), Config{Servers: map[string]ServerConfig{
		"s": {Type: TransportStdio, Command: "s"},
	}}, ManagerOptions{Dialer: &stubDialer{sessions: map[string]*stubSession{"s": session}}, CloseTimeout: time.Second})
	manager.Close()
	if !session.closed {
		t.Fatal("session should close")
	}
}

func TestManagerCloseTimeout(t *testing.T) {
	session := &blockingCloseSession{stubSession: stubSession{listTools: []RemoteTool{{Name: "a"}}}}
	manager := NewManager(context.Background(), Config{Servers: map[string]ServerConfig{
		"s": {Type: TransportStdio, Command: "s"},
	}}, ManagerOptions{Dialer: &customSessionDialer{session: session}, CloseTimeout: 10 * time.Millisecond})
	start := time.Now()
	manager.Close()
	if time.Since(start) > time.Second {
		t.Fatal("close did not return promptly")
	}
}

type stubDialer struct {
	mu       sync.Mutex
	sessions map[string]*stubSession
	errs     map[string]error
	calls    map[string]int
}

func (d *stubDialer) Dial(ctx context.Context, name string, cfg ServerConfig, info ClientInfo) (ClientSession, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.calls == nil {
		d.calls = map[string]int{}
	}
	d.calls[name]++
	if err := d.errs[name]; err != nil {
		return nil, err
	}
	return d.sessions[name], nil
}

type customSessionDialer struct {
	session ClientSession
}

func (d *customSessionDialer) Dial(ctx context.Context, name string, cfg ServerConfig, info ClientInfo) (ClientSession, error) {
	return d.session, nil
}

type blockingCloseSession struct {
	stubSession
}

func (s *blockingCloseSession) Close() error {
	select {}
}
