package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DiLRandI/just-a-todo/internal/todo"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestCreateListAndFilterTodos(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	_, err := st.CreateTodo(ctx, todo.CreateParams{
		Title:    "Pay rent",
		Priority: todo.PriorityHigh,
		DueDate:  "2026-05-13",
		Tags:     []string{"home", "money"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateTodo(ctx, todo.CreateParams{
		Title:    "Read docs",
		Priority: todo.PriorityLow,
		Tags:     []string{"learning"},
	})
	if err != nil {
		t.Fatal(err)
	}

	items, err := st.ListTodos(ctx, ListFilter{Tag: "home", Priority: todo.PriorityHigh})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "Pay rent" {
		t.Fatalf("filtered items = %#v", items)
	}

	noDue, err := st.ListTodos(ctx, ListFilter{NoDueOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(noDue) != 1 || noDue[0].Title != "Read docs" {
		t.Fatalf("no due items = %#v", noDue)
	}
}

func TestCompleteRecurringTodoCreatesNextOccurrence(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	item, err := st.CreateTodo(ctx, todo.CreateParams{
		Title:      "Water plants",
		Priority:   todo.PriorityNormal,
		DueDate:    "2026-05-01",
		RepeatRule: todo.RepeatWeekly,
		Tags:       []string{"home"},
	})
	if err != nil {
		t.Fatal(err)
	}

	done, next, err := st.CompleteTodo(ctx, item.ID, time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != todo.StatusDone {
		t.Fatalf("done status = %s", done.Status)
	}
	if next == nil {
		t.Fatal("expected next recurring todo")
	}
	if next.DueDate != "2026-05-15" {
		t.Fatalf("next due = %s", next.DueDate)
	}
	if len(next.Tags) != 1 || next.Tags[0] != "home" {
		t.Fatalf("next tags = %#v", next.Tags)
	}
}

func TestArchiveAndDelete(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	item, err := st.CreateTodo(ctx, todo.CreateParams{Title: "Old task", Priority: todo.PriorityNormal})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ArchiveTodo(ctx, item.ID); err != nil {
		t.Fatal(err)
	}

	open, err := st.ListTodos(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("open visible items = %#v", open)
	}

	archived, err := st.ListTodos(ctx, ListFilter{ArchivedOnly: true, IncludeArchived: true, AllStatuses: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 {
		t.Fatalf("archived items = %#v", archived)
	}

	if err := st.DeleteTodo(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	archived, err = st.ListTodos(ctx, ListFilter{ArchivedOnly: true, IncludeArchived: true, AllStatuses: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("deleted item still visible: %#v", archived)
	}
}
