package permission

import "testing"

func TestModeParsingNextAndFallback(t *testing.T) {
	if ParseMode("strict") != ModeStrict || ParseMode("bogus") != ModeDefault {
		t.Fatalf("parse mode failed")
	}
	sequence := []Mode{ModeStrict, ModeDefault, ModeAcceptEdits, ModeBypassPermissions, ModeStrict}
	for i := 0; i < len(sequence)-1; i++ {
		if got := NextMode(sequence[i]); got != sequence[i+1] {
			t.Fatalf("NextMode(%s) = %s, want %s", sequence[i], got, sequence[i+1])
		}
	}
	cases := []struct {
		mode     Mode
		category Category
		want     Decision
	}{
		{ModeStrict, CategoryRead, DecisionAsk},
		{ModeStrict, CategoryWrite, DecisionAsk},
		{ModeStrict, CategoryExec, DecisionAsk},
		{ModeDefault, CategoryRead, DecisionAllow},
		{ModeDefault, CategoryWrite, DecisionAsk},
		{ModeDefault, CategoryExec, DecisionAsk},
		{ModeAcceptEdits, CategoryRead, DecisionAllow},
		{ModeAcceptEdits, CategoryWrite, DecisionAllow},
		{ModeAcceptEdits, CategoryExec, DecisionAsk},
		{ModeBypassPermissions, CategoryRead, DecisionAllow},
		{ModeBypassPermissions, CategoryWrite, DecisionAllow},
		{ModeBypassPermissions, CategoryExec, DecisionAllow},
	}
	for _, tc := range cases {
		if got := modeFallback(tc.mode, tc.category); got != tc.want {
			t.Fatalf("modeFallback(%s, %s) = %s, want %s", tc.mode, tc.category, got, tc.want)
		}
	}
}
