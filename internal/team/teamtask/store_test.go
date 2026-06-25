package teamtask

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCreateGetListUpdate(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "tasks.json"))
	task, err := store.Create(context.Background(), CreateInput{Title: "Design", Assignee: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.Status != StatusTodo {
		t.Fatalf("task = %+v", task)
	}
	got, ok, err := store.Get(task.ID)
	if err != nil || !ok {
		t.Fatalf("Get = %+v, %v, %v", got, ok, err)
	}
	title := "Implement"
	status := StatusInProgress
	got, err = store.Update(context.Background(), task.ID, Patch{Title: &title, Status: &status})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != title || got.Status != status {
		t.Fatalf("updated = %+v", got)
	}
	list, err := store.List(ListFilter{Status: StatusInProgress})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].IsReady {
		t.Fatalf("list = %+v", list)
	}
}

func TestDependencyBidirectionalAndReady(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "tasks.json"))
	blocker, err := store.Create(context.Background(), CreateInput{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := store.Create(context.Background(), CreateInput{Title: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(context.Background(), blocked.ID, Patch{AddBlockedBy: []string{blocker.ID}}); err != nil {
		t.Fatal(err)
	}
	blockerGot, ok, err := store.Get(blocker.ID)
	if err != nil || !ok {
		t.Fatalf("Get blocker = %+v, %v, %v", blockerGot, ok, err)
	}
	if len(blockerGot.Blocks) != 1 || blockerGot.Blocks[0] != blocked.ID {
		t.Fatalf("blocker blocks = %#v", blockerGot.Blocks)
	}
	list, err := store.List(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	ready := readiness(list)
	if ready[blocked.ID] {
		t.Fatalf("blocked task should not be ready: %+v", list)
	}
	done := StatusDone
	if _, err := store.Update(context.Background(), blocker.ID, Patch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	list, err = store.List(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	ready = readiness(list)
	if !ready[blocked.ID] {
		t.Fatalf("blocked task should be ready after blocker done: %+v", list)
	}
	if _, err := store.Update(context.Background(), blocked.ID, Patch{RemoveBlockedBy: []string{blocker.ID}}); err != nil {
		t.Fatal(err)
	}
	blockerGot, _, err = store.Get(blocker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blockerGot.Blocks) != 0 {
		t.Fatalf("blocks not removed: %#v", blockerGot.Blocks)
	}
}

func TestAddBlocksMaintainsReverse(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "tasks.json"))
	a, err := store.Create(context.Background(), CreateInput{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Create(context.Background(), CreateInput{Title: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(context.Background(), a.ID, Patch{AddBlocks: []string{b.ID}}); err != nil {
		t.Fatal(err)
	}
	bGot, _, err := store.Get(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bGot.BlockedBy) != 1 || bGot.BlockedBy[0] != a.ID {
		t.Fatalf("blocked_by = %#v", bGot.BlockedBy)
	}
}

func readiness(tasks []ListedTask) map[string]bool {
	out := map[string]bool{}
	for _, task := range tasks {
		out[task.ID] = task.IsReady
	}
	return out
}
