package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/DiLRandI/just-a-todo/internal/todo"
)

func TestFormatLinePreservesArchivedCompletionState(t *testing.T) {
	now := time.Now()
	line := formatLine(todo.Todo{
		ID:         1,
		Title:      "finished",
		Priority:   todo.PriorityNormal,
		Status:     todo.StatusDone,
		ArchivedAt: &now,
	}, false)
	if !strings.Contains(line, "done+arc") {
		t.Fatalf("line = %q", line)
	}
}
