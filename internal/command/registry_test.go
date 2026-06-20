package command

import (
	"strings"
	"testing"
)

func noop(Context, Controller) error { return nil }

func TestRegistryLookupVisibleAndComplete(t *testing.T) {
	reg, err := NewRegistry([]Command{
		{Name: "/status", Aliases: []string{"/st"}, Description: "status", Usage: "/status", Handler: noop},
		{Name: "/session", Aliases: []string{"/sess"}, Description: "session", Usage: "/session", Handler: noop},
		{Name: "/secret", Kind: KindSkill, Hidden: true, Description: "secret", Usage: "/secret", Handler: noop},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd, ok := reg.Lookup("/ST"); !ok || cmd.Name != "/status" {
		t.Fatalf("lookup alias failed: %+v %v", cmd, ok)
	}
	visible := reg.Visible()
	if len(visible) != 2 || visible[0].Name != "/session" || visible[1].Name != "/status" {
		t.Fatalf("visible = %+v", visible)
	}
	items := reg.Complete("/s")
	if len(items) != 2 || items[0].Name != "/session" || items[1].Name != "/status" {
		t.Fatalf("complete = %+v", items)
	}
	if items := reg.Complete("/st"); len(items) != 1 || items[0].Text != "/status" {
		t.Fatalf("alias completion = %+v", items)
	}
	if items := reg.Complete("/sec"); len(items) != 0 {
		t.Fatalf("hidden completion = %+v", items)
	}
	if cmd, ok := reg.Lookup("/secret"); !ok || cmd.Kind != KindSkill {
		t.Fatalf("hidden skill command lookup failed: %+v %v", cmd, ok)
	}
}

func TestRegistryDynamicRegisterRemoveAndHas(t *testing.T) {
	reg := MustNewRegistry([]Command{{Name: "/help", Handler: noop}})
	if err := reg.Register(Command{Name: "/demo", Kind: KindSkill, Skill: true, Handler: noop}); err != nil {
		t.Fatal(err)
	}
	if !reg.Has("/demo") {
		t.Fatal("missing dynamic command")
	}
	items := reg.Complete("/d")
	if len(items) != 1 || !items[0].Skill || items[0].Kind != KindSkill {
		t.Fatalf("items = %+v", items)
	}
	RemoveSkillCommands(reg)
	if reg.Has("/demo") {
		t.Fatal("skill command not removed")
	}
}

func TestRegistryConflicts(t *testing.T) {
	tests := []struct {
		name     string
		commands []Command
		want     string
	}{
		{name: "main", want: "/status", commands: []Command{
			{Name: "/status", Handler: noop},
			{Name: "/STATUS", Handler: noop},
		}},
		{name: "alias", want: "/st", commands: []Command{
			{Name: "/status", Aliases: []string{"/st"}, Handler: noop},
			{Name: "/session", Aliases: []string{"/ST"}, Handler: noop},
		}},
		{name: "cross", want: "/foo", commands: []Command{
			{Name: "/status", Aliases: []string{"/foo"}, Handler: noop},
			{Name: "/foo", Handler: noop},
		}},
		{name: "internal", want: "/foo", commands: []Command{
			{Name: "/foo", Aliases: []string{"/FOO"}, Handler: noop},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRegistry(tt.commands)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want token %s", err, tt.want)
			}
		})
	}
}

func TestBuiltins(t *testing.T) {
	reg := NewBuiltinRegistry()
	visible := reg.Visible()
	want := []string{"/clear", "/compact", "/do", "/help", "/memory", "/permission", "/plan", "/session", "/skill", "/status"}
	if len(visible) != len(want) {
		t.Fatalf("visible count = %d", len(visible))
	}
	for i, name := range want {
		if visible[i].Name != name || visible[i].Description == "" || visible[i].Usage == "" {
			t.Fatalf("visible[%d] = %+v", i, visible[i])
		}
	}
	kinds := map[string]Kind{}
	for _, cmd := range visible {
		kinds[cmd.Name] = cmd.Kind
	}
	for _, name := range []string{"/help", "/memory", "/permission", "/session", "/skill", "/status"} {
		if kinds[name] != KindLocal {
			t.Fatalf("%s kind = %v", name, kinds[name])
		}
	}
	for _, name := range []string{"/clear", "/compact", "/do", "/plan"} {
		if kinds[name] != KindUI {
			t.Fatalf("%s kind = %v", name, kinds[name])
		}
	}
}
