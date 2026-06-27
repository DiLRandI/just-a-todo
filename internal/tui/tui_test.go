package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DiLRandI/just-a-todo/internal/dateparse"
	"github.com/DiLRandI/just-a-todo/internal/store"
	"github.com/DiLRandI/just-a-todo/internal/todo"
)

func testModel(t *testing.T, color bool) Model {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewModel(context.Background(), st, color)
}

func TestExactSuggestionSelectsTypedValue(t *testing.T) {
	items := filteredSuggestions(dueSuggestions(testTime()), "tomorrow")
	if len(items) != 1 || items[0].value != "tomorrow" {
		t.Fatalf("suggestions = %#v", items)
	}
}

func TestRenderTodoLineTruncatesUnicodeSafely(t *testing.T) {
	line := renderTodoLine(todo.Todo{ID: 1, Title: "買い物🙂", Priority: todo.PriorityNormal}, 32)
	if !strings.Contains(line, "...") {
		t.Fatalf("line was not truncated: %q", line)
	}
	if !utf8.ValidString(line) {
		t.Fatalf("line contains invalid UTF-8: %q", line)
	}
}

func TestNoColorViewContainsNoANSI(t *testing.T) {
	model := testModel(t, false)
	views := []string{model.View().Content}
	model.startCreate()
	views = append(views, model.View().Content)
	for _, view := range views {
		if strings.Contains(view, "\x1b[") {
			t.Fatalf("view contains ANSI escapes: %q", view)
		}
	}
}

func TestCurrentDateRangeIncludesEarlierOverdueTodos(t *testing.T) {
	model := testModel(t, false)
	var filter store.ListFilter
	model.applyDateRange(&filter, "today", testTime(), true)
	if filter.DueStart != "" || filter.DueEnd != "2026-05-13" {
		t.Fatalf("filter = %#v", filter)
	}
	model.applyDateRange(&filter, "next week", testTime(), false)
	if filter.DueStart != "2026-05-18" || filter.DueEnd != "2026-05-24" {
		t.Fatalf("next-week filter = %#v", filter)
	}
}

func TestTomorrowRangeExcludesNonOverdueTodayItems(t *testing.T) {
	now := testTime()
	r, err := dateparse.RangeFor("tomorrow", now)
	if err != nil {
		t.Fatal(err)
	}
	items := []todo.Todo{
		{ID: 1, DueDate: "2026-05-12"},
		{ID: 2, DueDate: "2026-05-13"},
		{ID: 3, DueDate: "2026-05-13", DueTime: "09:00"},
		{ID: 4, DueDate: "2026-05-14"},
	}
	got := itemsForRange(items, r, now)
	if len(got) != 3 || got[0].ID != 1 || got[1].ID != 3 || got[2].ID != 4 {
		t.Fatalf("items = %#v", got)
	}
}

func testTime() time.Time {
	return time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
}
