package tools

import "testing"

func TestDefinitionsFiltered(t *testing.T) {
	r, err := NewRegistry(
		fakeTool{name: "ordinary", safety: SafetySideEffect},
		fakeTool{name: "readonly", safety: SafetyReadOnly},
		fakeTool{name: "system", safety: SafetyReadOnly, system: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if defs := r.DefinitionsFiltered(nil); len(defs) != 3 {
		t.Fatalf("defs = %+v", defs)
	}
	defs := r.DefinitionsFiltered([]string{"ordinary"})
	names := namesOf(defs)
	if len(names) != 2 || names[0] != "ordinary" || names[1] != "system" {
		t.Fatalf("filtered = %+v", names)
	}
	if !r.IsSystem("system") || r.IsSystem("ordinary") {
		t.Fatalf("system query failed")
	}
	if err := r.RegisterOrReplace(fakeTool{name: "ordinary", safety: SafetyReadOnly}); err != nil {
		t.Fatal(err)
	}
	if safety, ok := r.Safety("ordinary"); !ok || safety != SafetyReadOnly {
		t.Fatalf("replace failed: %s %v", safety, ok)
	}
}
