package team

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"PseudoClaude/internal/agent"
	"PseudoClaude/internal/conversation"
	"PseudoClaude/internal/llm"
	"PseudoClaude/internal/task"
	"PseudoClaude/internal/team/mailbox"
	"PseudoClaude/internal/tools"
)

func TestSpawnMemberCreatesStateMailboxAndWorktree(t *testing.T) {
	home := t.TempDir()
	tasks := task.NewManager(task.Options{Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		if runner.CWD == "" || runner.Team == nil || runner.Env.Team == nil {
			t.Fatalf("runner missing team context: %+v env=%+v", runner.Team, runner.Env.Team)
		}
		return agent.CompletionResult{Text: "member done", Stop: agent.Stop{Reason: agent.StopCompleted}}
	}})
	registry, err := tools.NewRegistry(fakeTeamTool{name: "TaskList"}, fakeTeamTool{name: "SendMessage"})
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(ManagerOptions{HomeDir: home, Worktrees: &fakeWorktrees{}, Tasks: tasks})
	if err != nil {
		t.Fatal(err)
	}
	created, err := mgr.Create(context.Background(), CreateInput{Name: "Demo", LeadAgentID: "lead-a"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := mgr.SpawnMember(context.Background(), agent.TeamLaunchInput{
		TeamName:         "Demo",
		MemberName:       "Alice",
		Prompt:           "Do the work",
		Description:      "initial task",
		SubagentType:     "general-purpose",
		PlanModeRequired: true,
		Parent: agent.RunnerSnapshot{
			Provider: fakeTeamProvider{},
			Registry: registry,
			Env:      tools.Env{},
			Config:   agent.DefaultConfig(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.MemberName != "alice" || result.AgentID == "" || result.WorktreePath == "" || result.SessionDir == "" {
		t.Fatalf("result = %+v", result)
	}
	reloaded, err := loadTeam(created.ConfigPath, home)
	if err != nil {
		t.Fatal(err)
	}
	member, ok := reloaded.MemberByName("alice")
	if !ok || !member.PlanModeRequired || member.WorktreeName == "" {
		t.Fatalf("member = %+v ok=%v", member, ok)
	}
	store, err := mgr.MailboxStore("Demo")
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := store.Read(result.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Content != "Do the work" || msgs[0].Read {
		t.Fatalf("messages = %#v", msgs)
	}
	waitTeamTaskDone(t, tasks, result.AgentID)
	waitMemberIdleAndLeadMessage(t, created.ConfigPath, home, store, result.AgentID)
}

func TestSpawnMemberInProcessRequiresTaskManager(t *testing.T) {
	home := t.TempDir()
	registry, err := tools.NewRegistry(fakeTeamTool{name: "SendMessage"})
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(ManagerOptions{HomeDir: home, Worktrees: &fakeWorktrees{}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := mgr.Create(context.Background(), CreateInput{Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.SpawnMember(context.Background(), agent.TeamLaunchInput{
		TeamName:   "Demo",
		MemberName: "Alice",
		Prompt:     "Do the work",
		Parent: agent.RunnerSnapshot{
			Provider: fakeTeamProvider{},
			Registry: registry,
			Env:      tools.Env{},
			Config:   agent.DefaultConfig(),
		},
	})
	if err != ErrBackendDisabled {
		t.Fatalf("SpawnMember err = %v, want ErrBackendDisabled", err)
	}
	reloaded, err := loadTeam(created.ConfigPath, home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.MemberByName("alice"); ok {
		t.Fatalf("member should not be registered when backend is unavailable: %+v", reloaded.Members)
	}
}

func TestResumeMemberContinuesIdleInProcessTask(t *testing.T) {
	home := t.TempDir()
	var mu sync.Mutex
	prompts := []string{}
	tasks := task.NewManager(task.Options{IDSource: atomicTaskIDs(), Run: func(ctx context.Context, runner agent.Runner, conv *conversation.Conversation, prompt string) agent.CompletionResult {
		mu.Lock()
		prompts = append(prompts, prompt)
		mu.Unlock()
		return agent.CompletionResult{Text: "done " + prompt, Stop: agent.Stop{Reason: agent.StopCompleted}}
	}})
	registry, err := tools.NewRegistry(fakeTeamTool{name: "SendMessage"})
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(ManagerOptions{HomeDir: home, Worktrees: &fakeWorktrees{}, Tasks: tasks})
	if err != nil {
		t.Fatal(err)
	}
	created, err := mgr.Create(context.Background(), CreateInput{Name: "Demo", LeadAgentID: "lead-a"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := mgr.SpawnMember(context.Background(), agent.TeamLaunchInput{
		TeamName:   "Demo",
		MemberName: "Alice",
		Prompt:     "initial",
		Parent: agent.RunnerSnapshot{
			Provider: fakeTeamProvider{},
			Registry: registry,
			Env:      tools.Env{},
			Config:   agent.DefaultConfig(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := mgr.MailboxStore("Demo")
	if err != nil {
		t.Fatal(err)
	}
	waitTeamTaskDone(t, tasks, result.AgentID)
	waitMemberIdleAndLeadMessageCount(t, created.ConfigPath, home, store, result.AgentID, 1)
	if err := store.Write(context.Background(), result.AgentID, mailbox.Message{From: "lead-a", To: result.AgentID, Type: mailbox.MessageText, Summary: "follow-up", Content: "continue"}); err != nil {
		t.Fatal(err)
	}
	nextID, err := mgr.ResumeMember(context.Background(), "Demo", result.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	waitTeamTaskDone(t, tasks, nextID)
	waitMemberIdleAndLeadMessageCount(t, created.ConfigPath, home, store, result.AgentID, 2)
	mu.Lock()
	defer mu.Unlock()
	if len(prompts) != 2 || prompts[0] != "initial" || !strings.Contains(prompts[1], "new unread team mailbox messages") {
		t.Fatalf("prompts = %#v", prompts)
	}
}

func waitTeamTaskDone(t *testing.T, tasks *task.Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, ok := tasks.Get(id)
		if ok && snap.Status != task.StatusRunning {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for team task %s", id)
}

func waitMemberIdleAndLeadMessage(t *testing.T, configPath, home string, store interface {
	Read(string) ([]mailbox.Message, error)
}, agentID string) {
	t.Helper()
	waitMemberIdleAndLeadMessageCount(t, configPath, home, store, agentID, 1)
}

func waitMemberIdleAndLeadMessageCount(t *testing.T, configPath, home string, store interface {
	Read(string) ([]mailbox.Message, error)
}, agentID string, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		reloaded, err := loadTeam(configPath, home)
		if err != nil {
			t.Fatal(err)
		}
		member, ok := reloaded.MemberByAgentID(agentID)
		leadMsgs, msgErr := store.Read("lead-a")
		if ok && member.IsActive != nil && !*member.IsActive && msgErr == nil && len(leadMsgs) == count && leadMsgs[count-1].From == agentID && leadMsgs[count-1].Summary != "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for idle member and %d lead notification(s)", count)
}

func atomicTaskIDs() task.IDSource {
	var n int
	return func() string {
		n++
		return "resume-task-" + string(rune('0'+n))
	}
}

type fakeTeamProvider struct{}

func (fakeTeamProvider) Name() string  { return "fake" }
func (fakeTeamProvider) Model() string { return "fake" }
func (fakeTeamProvider) Stream(ctx context.Context, req llm.Request) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch
}

type fakeTeamTool struct {
	name string
}

func (t fakeTeamTool) Definition() tools.Definition {
	return tools.Definition{Name: t.name, Safety: tools.SafetyReadOnly}
}

func (t fakeTeamTool) Execute(context.Context, json.RawMessage, tools.Env) tools.Result {
	return tools.Success(t.name, "", nil)
}
