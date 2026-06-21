package hook

import "testing"

func TestPromptQueue(t *testing.T) {
	q := NewPromptQueue()
	q.Add(" one ", "", "two")
	got := q.Drain()
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("drain = %+v", got)
	}
	if got := q.Drain(); len(got) != 0 {
		t.Fatalf("second drain = %+v", got)
	}
	q.Add("again")
	q.Clear()
	if got := q.Drain(); len(got) != 0 {
		t.Fatalf("after clear = %+v", got)
	}
}
