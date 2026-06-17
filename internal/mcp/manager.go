package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"PseudoClaude/internal/tools"
)

func NewManager(ctx context.Context, cfg Config, opts ManagerOptions) *Manager {
	opts = normalizeOptions(opts)
	manager := &Manager{closeTimeout: opts.CloseTimeout, stats: Stats{Configured: len(cfg.Servers)}}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(cfg.Servers) == 0 {
		return manager
	}

	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, name := range names {
		name := name
		server := cfg.Servers[name]
		wg.Add(1)
		go func() {
			defer wg.Done()
			connectCtx, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
			defer cancel()

			session, err := opts.Dialer.Dial(connectCtx, name, server, opts.ClientInfo)
			if err != nil {
				mu.Lock()
				manager.issues = append(manager.issues, Issue{Server: name, Stage: "connect", Message: err.Error()})
				mu.Unlock()
				return
			}
			remoteTools, err := session.ListTools(connectCtx)
			if err != nil {
				_ = session.Close()
				mu.Lock()
				manager.issues = append(manager.issues, Issue{Server: name, Stage: "list_tools", Message: err.Error()})
				mu.Unlock()
				return
			}
			mu.Lock()
			manager.stats.Connected++
			manager.stats.Discovered += len(remoteTools)
			mu.Unlock()
			sort.SliceStable(remoteTools, func(i, j int) bool {
				return remoteTools[i].Name < remoteTools[j].Name
			})
			adapted := make([]tools.Tool, 0, len(remoteTools))
			seen := map[string]bool{}
			var issues []Issue
			for _, remote := range remoteTools {
				if server.ReadOnly {
					remote.ReadOnly = true
				}
				fullName := FullToolName(name, remote.Name)
				if seen[fullName] {
					issues = append(issues, Issue{Server: name, Tool: remote.Name, Stage: "adapt_tool", Message: fmt.Sprintf("重复的 MCP 工具名 %q，已跳过", fullName)})
					continue
				}
				seen[fullName] = true
				tool, issue := AdaptTool(name, remote, session, opts.CallTimeout)
				if issue != nil {
					issues = append(issues, *issue)
					continue
				}
				adapted = append(adapted, tool)
			}

			mu.Lock()
			manager.sessions = append(manager.sessions, &serverSession{name: name, session: session})
			manager.tools = append(manager.tools, adapted...)
			manager.stats.Adapted += len(adapted)
			for _, tool := range adapted {
				switch tool.Definition().Safety {
				case tools.SafetyReadOnly:
					manager.stats.ReadOnly++
				case tools.SafetySideEffect:
					manager.stats.SideEffect++
				}
			}
			manager.issues = append(manager.issues, issues...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.SliceStable(manager.tools, func(i, j int) bool {
		return manager.tools[i].Definition().Name < manager.tools[j].Definition().Name
	})
	sort.SliceStable(manager.issues, func(i, j int) bool {
		if manager.issues[i].Server != manager.issues[j].Server {
			return manager.issues[i].Server < manager.issues[j].Server
		}
		if manager.issues[i].Stage != manager.issues[j].Stage {
			return manager.issues[i].Stage < manager.issues[j].Stage
		}
		return manager.issues[i].Tool < manager.issues[j].Tool
	})
	return manager
}

func (m *Manager) Tools() []tools.Tool {
	if m == nil {
		return nil
	}
	return append([]tools.Tool(nil), m.tools...)
}

func (m *Manager) Issues() []Issue {
	if m == nil {
		return nil
	}
	return append([]Issue(nil), m.issues...)
}

func (m *Manager) Stats() Stats {
	if m == nil {
		return Stats{}
	}
	return m.stats
}

func (m *Manager) Close() {
	if m == nil || len(m.sessions) == 0 {
		return
	}
	timeout := m.closeTimeout
	if timeout <= 0 {
		timeout = defaultCloseTimeout
	}
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for _, server := range m.sessions {
			server := server
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = server.session.Close()
			}()
		}
		wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

func normalizeOptions(opts ManagerOptions) ManagerOptions {
	if opts.ConnectTimeout <= 0 {
		opts.ConnectTimeout = defaultConnectTimeout
	}
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = defaultCallTimeout
	}
	if opts.CloseTimeout <= 0 {
		opts.CloseTimeout = defaultCloseTimeout
	}
	if opts.ClientInfo.Name == "" {
		opts.ClientInfo.Name = "PseudoClaude"
	}
	if opts.ClientInfo.Version == "" {
		opts.ClientInfo.Version = "dev"
	}
	if opts.Dialer == nil {
		opts.Dialer = SDKDialer{}
	}
	return opts
}
