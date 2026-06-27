package store

import (
	"context"
	"errors"
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

func TestRecurringTodoCannotReopenWhileSuccessorExists(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	item, err := st.CreateTodo(ctx, todo.CreateParams{
		Title:      "Water plants",
		DueDate:    "2026-05-01",
		RepeatRule: todo.RepeatWeekly,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, next, err := st.CompleteTodo(ctx, item.ID, time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReopenTodo(ctx, item.ID); !errors.Is(err, ErrRecurringSuccessorExists) {
		t.Fatalf("reopen error = %v", err)
	}
	if err := st.DeleteTodo(ctx, next.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.ReopenTodo(ctx, item.ID); err != nil {
		t.Fatalf("reopen after deleting successor: %v", err)
	}
}

func TestMonthlyRecurrencePreservesAnchorAcrossShortMonth(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	item, err := st.CreateTodo(ctx, todo.CreateParams{
		Title:      "Month end",
		DueDate:    "2026-01-31",
		RepeatRule: todo.RepeatMonthly,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, february, err := st.CompleteTodo(ctx, item.ID, time.Date(2026, 2, 20, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if february.DueDate != "2026-02-28" || february.RecurrenceAnchorDay != 31 {
		t.Fatalf("february occurrence = %#v", february)
	}
	_, march, err := st.CompleteTodo(ctx, february.ID, time.Date(2026, 2, 28, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if march.DueDate != "2026-03-31" {
		t.Fatalf("march due = %s", march.DueDate)
	}
}

func TestSearchTreatsLikeWildcardsLiterally(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, title := range []string{"100% complete", "under_score", "ordinary"} {
		if _, err := st.CreateTodo(ctx, todo.CreateParams{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	for search, want := range map[string]string{"%": "100% complete", "_": "under_score"} {
		items, err := st.ListTodos(ctx, ListFilter{Search: search})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].Title != want {
			t.Fatalf("search %q = %#v", search, items)
		}
	}
}

func TestMigrateIsVersionedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("migration count = %d, want 2", count)
	}
}

func TestMigrateUpgradesVersionOneDatabase(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
		INSERT INTO schema_migrations(version, applied_at) VALUES (1, '2026-01-01T00:00:00Z');
		CREATE TABLE todos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			notes TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			priority TEXT NOT NULL DEFAULT 'normal',
			due_date TEXT NULL,
			due_time TEXT NULL,
			repeat_rule TEXT NOT NULL DEFAULT 'none',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT NULL,
			archived_at TEXT NULL
		);
		INSERT INTO todos(title, due_date, repeat_rule, created_at, updated_at)
		VALUES ('Month end', '2026-01-31', 'monthly', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
	`); err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var anchor int
	if err := st.db.QueryRowContext(ctx, `SELECT recurrence_anchor_day FROM todos WHERE id = 1`).Scan(&anchor); err != nil {
		t.Fatal(err)
	}
	if anchor != 31 {
		t.Fatalf("anchor = %d, want 31", anchor)
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
