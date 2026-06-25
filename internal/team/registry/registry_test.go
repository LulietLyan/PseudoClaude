package registry

import "testing"

func TestRegistryRegisterResolve(t *testing.T) {
	r := New()
	r.Register("alice", "agent-a")
	if got, ok := r.Resolve("alice"); !ok || got != "agent-a" {
		t.Fatalf("Resolve(name) = %q, %v", got, ok)
	}
	if got, ok := r.Resolve("agent-a"); !ok || got != "agent-a" {
		t.Fatalf("Resolve(agentID) = %q, %v", got, ok)
	}
	if got, ok := r.NameOf("agent-a"); !ok || got != "alice" {
		t.Fatalf("NameOf = %q, %v", got, ok)
	}
}

func TestRegistryOverwriteName(t *testing.T) {
	r := New()
	r.Register("alice", "agent-a")
	r.Register("alice", "agent-b")
	if _, ok := r.NameOf("agent-a"); ok {
		t.Fatal("old agent reverse mapping remains")
	}
	if got, ok := r.Resolve("alice"); !ok || got != "agent-b" {
		t.Fatalf("Resolve = %q, %v", got, ok)
	}
}

func TestRegistryRenameAgent(t *testing.T) {
	r := New()
	r.Register("alice", "agent-a")
	r.Register("bob", "agent-a")
	if _, ok := r.Resolve("alice"); ok {
		t.Fatal("old name mapping remains")
	}
	if got, ok := r.NameOf("agent-a"); !ok || got != "bob" {
		t.Fatalf("NameOf = %q, %v", got, ok)
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := New()
	r.Register("alice", "agent-a")
	r.Unregister("alice")
	if _, ok := r.Resolve("alice"); ok {
		t.Fatal("name still resolves")
	}
	r.Register("bob", "agent-b")
	r.UnregisterByAgentID("agent-b")
	if _, ok := r.Resolve("bob"); ok {
		t.Fatal("agent still resolves")
	}
}
