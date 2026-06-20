package skills

import "testing"

func TestActiveSkills(t *testing.T) {
	active := NewActiveSkills()
	active.Activate("one", "a")
	if names := active.Names(); len(names) != 1 || names[0] != "one" {
		t.Fatalf("names = %+v", names)
	}
	active.Activate("one", "b")
	active.Activate("two", "c")
	snapshot := active.Snapshot()
	if len(snapshot) != 2 || snapshot[0].Body != "b" || snapshot[1].Name != "two" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	active.Clear()
	if len(active.Names()) != 0 {
		t.Fatalf("not cleared: %+v", active.Names())
	}
}
