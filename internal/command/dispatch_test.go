package command

import (
	"errors"
	"strings"
	"testing"
)

type fakeController struct {
	idle         bool
	messages     []string
	messageKind  []MessageKind
	workMode     WorkMode
	preset       string
	runSkill     string
	clearSkills  bool
	compact      bool
	clear        bool
	reloadAgents bool
	teams        []TeamSummary
	teamDetail   TeamDetail
	deletedTeam  string
	killedTeam   string
	killedMember string
}

func (f *fakeController) Show(kind MessageKind, text string) {
	f.messageKind = append(f.messageKind, kind)
	f.messages = append(f.messages, text)
}
func (f *fakeController) IsIdle() bool { return f.idle }
func (f *fakeController) WorkMode() WorkMode {
	if f.workMode == "" {
		return WorkModeDefault
	}
	return f.workMode
}
func (f *fakeController) SetWorkMode(mode WorkMode)              { f.workMode = mode }
func (f *fakeController) PermissionMode() string                 { return "default" }
func (f *fakeController) Status() StatusInfo                     { return StatusInfo{WorkMode: f.WorkMode()} }
func (f *fakeController) Session() SessionInfo                   { return SessionInfo{} }
func (f *fakeController) MemorySummary() string                  { return "" }
func (f *fakeController) TriggerCompact()                        { f.compact = true }
func (f *fakeController) ClearScreen()                           { f.clear = true }
func (f *fakeController) SendPresetUserMessage(_, prompt string) { f.preset = prompt }
func (f *fakeController) RefreshStatus()                         {}
func (f *fakeController) ListSkills() []SkillSummary             { return nil }
func (f *fakeController) ListHooks() []HookSummary               { return nil }
func (f *fakeController) HookSources() []string                  { return nil }
func (f *fakeController) ListAgents() []AgentSummary {
	return []AgentSummary{{Name: "explore", Description: "Explore.", Source: "builtin", Model: "haiku"}}
}
func (f *fakeController) DescribeAgent(name string) (AgentDetail, bool) {
	if name != "explore" {
		return AgentDetail{}, false
	}
	return AgentDetail{Active: AgentSummary{Name: "explore", Description: "Explore.", Source: "builtin", Model: "haiku"}, Prompt: "Prompt."}, true
}
func (f *fakeController) ReloadAgents() { f.reloadAgents = true }
func (f *fakeController) RunSkill(name, args string) error {
	f.runSkill = name + ":" + args
	return nil
}
func (f *fakeController) ReloadSkills()      {}
func (f *fakeController) ClearActiveSkills() { f.clearSkills = true }
func (f *fakeController) TeamAvailable() bool {
	return f.teams != nil || f.teamDetail.Name != ""
}
func (f *fakeController) ListTeams() []TeamSummary { return f.teams }
func (f *fakeController) TeamInfo(name string) (TeamDetail, bool) {
	if f.teamDetail.Name == name {
		return f.teamDetail, true
	}
	return TeamDetail{}, false
}
func (f *fakeController) DeleteTeam(name string, force bool) error {
	f.deletedTeam = name
	return nil
}
func (f *fakeController) KillTeamMember(teamName, memberName string) error {
	f.killedTeam = teamName
	f.killedMember = memberName
	return nil
}

func TestDispatch(t *testing.T) {
	reg := NewBuiltinRegistry()
	ctl := &fakeController{idle: true}
	if got := Dispatch(reg, "hello", ctl); got.Handled {
		t.Fatalf("plain input handled")
	}
	if got := Dispatch(reg, "   ", ctl); !got.Handled {
		t.Fatalf("empty input not handled")
	}
	if got := Dispatch(reg, "/unknown", ctl); !got.Handled || len(ctl.messages) == 0 || !strings.Contains(ctl.messages[len(ctl.messages)-1], "/help") {
		t.Fatalf("unknown result=%+v messages=%+v", got, ctl.messages)
	}
	if ctl.messageKind[len(ctl.messageKind)-1] != MessageHelp {
		t.Fatalf("unknown message kind = %s", ctl.messageKind[len(ctl.messageKind)-1])
	}
	ctl = &fakeController{idle: true}
	if got := Dispatch(reg, "/HELP", ctl); !got.Handled || len(ctl.messages) == 0 || !strings.Contains(ctl.messages[0], "/status") {
		t.Fatalf("help failed result=%+v messages=%+v", got, ctl.messages)
	}
	if ctl.messageKind[0] != MessageHelp {
		t.Fatalf("help message kind = %s", ctl.messageKind[0])
	}
	ctl = &fakeController{idle: true}
	if got := Dispatch(reg, "/help extra", ctl); !got.Handled || len(ctl.messages) == 0 || !strings.Contains(ctl.messages[0], "/help") || !strings.Contains(ctl.messages[0], "Invalid usage") {
		t.Fatalf("invalid usage result=%+v messages=%+v", got, ctl.messages)
	}
	ctl = &fakeController{idle: false}
	if got := Dispatch(reg, "/compact", ctl); !got.Handled || ctl.compact || !strings.Contains(ctl.messages[0], "wait") {
		t.Fatalf("non-idle ui command result=%+v ctl=%+v", got, ctl)
	}
	if ctl.messageKind[0] != MessageHelp {
		t.Fatalf("non-idle message kind = %s", ctl.messageKind[0])
	}
	if got := Dispatch(reg, "/status", ctl); !got.Handled || len(ctl.messages) < 2 {
		t.Fatalf("local non-idle failed result=%+v messages=%+v", got, ctl.messages)
	}
}

func TestDispatchHandlerError(t *testing.T) {
	want := errors.New("boom")
	reg := MustNewRegistry([]Command{{Name: "/x", Kind: KindLocal, Handler: func(Context, Controller) error { return want }}})
	got := Dispatch(reg, "/x", &fakeController{idle: true})
	if got.Err != want {
		t.Fatalf("err = %v", got.Err)
	}
}

func TestDispatchHiddenSkillCommand(t *testing.T) {
	called := false
	reg := MustNewRegistry([]Command{
		{Name: "/skill:review", Kind: KindSkill, Hidden: true, Handler: func(Context, Controller) error {
			called = true
			return nil
		}},
	})
	ctl := &fakeController{idle: true}
	got := Dispatch(reg, "/skill:review", ctl)
	if !got.Handled || got.Kind != KindSkill || !called {
		t.Fatalf("skill dispatch result=%+v called=%v", got, called)
	}
	if visible := reg.Visible(); len(visible) != 0 {
		t.Fatalf("hidden skill command should not be visible: %+v", visible)
	}
	if items := reg.Complete("/skill"); len(items) != 0 {
		t.Fatalf("hidden skill command should not complete: %+v", items)
	}

	called = false
	ctl = &fakeController{idle: false}
	got = Dispatch(reg, "/skill:review", ctl)
	if !got.Handled || got.Kind != KindSkill || called || len(ctl.messages) == 0 || !strings.Contains(ctl.messages[0], "wait") {
		t.Fatalf("non-idle skill dispatch result=%+v called=%v messages=%+v", got, called, ctl.messages)
	}
}

func TestDispatchRegisteredSkillCommand(t *testing.T) {
	reg := NewBuiltinRegistry()
	errs := RegisterSkillCommands(reg, []SkillSummary{{Name: "demo", Description: "Demo skill."}})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	ctl := &fakeController{idle: true}
	got := Dispatch(reg, "/demo hello", ctl)
	if !got.Handled || got.Kind != KindSkill || ctl.runSkill != "demo:hello" {
		t.Fatalf("got=%+v ctl=%+v", got, ctl)
	}
}

func TestAgentsCommand(t *testing.T) {
	reg := NewBuiltinRegistry()
	ctl := &fakeController{idle: true}
	got := Dispatch(reg, "/agents", ctl)
	if !got.Handled || len(ctl.messages) == 0 || !strings.Contains(ctl.messages[0], "explore") {
		t.Fatalf("/agents result=%+v messages=%+v", got, ctl.messages)
	}
	ctl = &fakeController{idle: true}
	got = Dispatch(reg, "/agents reload", ctl)
	if !got.Handled || !ctl.reloadAgents {
		t.Fatalf("/agents reload result=%+v ctl=%+v", got, ctl)
	}
	ctl = &fakeController{idle: true}
	got = Dispatch(reg, "/agents explore", ctl)
	if !got.Handled || len(ctl.messages) == 0 || !strings.Contains(ctl.messages[0], "Prompt.") {
		t.Fatalf("/agents explore result=%+v messages=%+v", got, ctl.messages)
	}
}

func TestTeamCommand(t *testing.T) {
	reg := NewBuiltinRegistry()
	ctl := &fakeController{idle: true, teams: []TeamSummary{{Name: "demo", SanitizedName: "demo", Backend: "in-process", MemberCount: 2, ActiveCount: 1}}}
	got := Dispatch(reg, "/team", ctl)
	if !got.Handled || len(ctl.messages) == 0 || !strings.Contains(ctl.messages[0], "demo") {
		t.Fatalf("/team result=%+v messages=%+v", got, ctl.messages)
	}
	ctl = &fakeController{idle: true, teamDetail: TeamDetail{TeamSummary: TeamSummary{Name: "demo", SanitizedName: "demo", Backend: "in-process"}, Members: []TeamMemberSummary{{Name: "alice", AgentID: "agent-a"}}}}
	got = Dispatch(reg, "/team info demo", ctl)
	if !got.Handled || len(ctl.messages) == 0 || !strings.Contains(ctl.messages[0], "alice") {
		t.Fatalf("/team info result=%+v messages=%+v", got, ctl.messages)
	}
	ctl = &fakeController{idle: true, teams: []TeamSummary{}}
	got = Dispatch(reg, "/team delete demo --force", ctl)
	if !got.Handled || ctl.deletedTeam != "demo" {
		t.Fatalf("/team delete result=%+v ctl=%+v", got, ctl)
	}
	ctl = &fakeController{idle: true, teams: []TeamSummary{}}
	got = Dispatch(reg, "/team kill demo alice", ctl)
	if !got.Handled || ctl.killedTeam != "demo" || ctl.killedMember != "alice" {
		t.Fatalf("/team kill result=%+v ctl=%+v", got, ctl)
	}
}
