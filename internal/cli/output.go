package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/DiLRandI/just-a-todo/internal/dateparse"
	"github.com/DiLRandI/just-a-todo/internal/todo"
)

func printDateSummary(w io.Writer, r dateparse.Range, items []todo.Todo, now time.Time, color bool) {
	var overdue []todo.Todo
	var current []todo.Todo
	for _, item := range items {
		if item.DueDate == "" {
			continue
		}
		switch {
		case dateparse.IsOverdue(item.DueDate, item.DueTime, now):
			overdue = append(overdue, item)
		case dateparse.InRange(item.DueDate, r):
			current = append(current, item)
		}
	}

	title := map[string]string{
		"today":    "Today",
		"tomorrow": "Tomorrow",
		"week":     "This week",
		"month":    "This month",
	}[r.Name]

	if len(overdue) == 0 && len(current) == 0 {
		fmt.Fprintf(w, "No todos for %s.\n", strings.ToLower(title))
		return
	}

	fmt.Fprintln(w, heading(title, color))
	if len(overdue) > 0 {
		printGroup(w, "Overdue", overdue, color)
	}
	if len(current) > 0 {
		printGroup(w, title, current, color)
	}
}

func printSimpleList(w io.Writer, title string, items []todo.Todo, color bool) {
	if len(items) == 0 {
		fmt.Fprintf(w, "No %s.\n", strings.ToLower(title))
		return
	}
	fmt.Fprintln(w, heading(title, color))
	printGroup(w, "", items, color)
}

func printGroup(w io.Writer, title string, items []todo.Todo, color bool) {
	if title != "" {
		fmt.Fprintf(w, "\n%s (%d)\n", section(title, color), len(items))
	}
	for _, item := range items {
		fmt.Fprintf(w, "  %s\n", formatLine(item, color))
	}
}

func formatLine(item todo.Todo, color bool) string {
	priority := string(item.Priority)
	if color {
		switch item.Priority {
		case todo.PriorityHigh:
			priority = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render(priority)
		case todo.PriorityLow:
			priority = lipgloss.NewStyle().Faint(true).Render(priority)
		default:
			priority = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render(priority)
		}
	}

	status := "open"
	if item.Status == todo.StatusDone {
		status = "done"
	}
	if item.IsArchived() {
		status = "archived"
	}

	tags := ""
	if len(item.Tags) > 0 {
		tags = " [" + strings.Join(item.Tags, ", ") + "]"
	}

	return fmt.Sprintf("#%-4d %-8s %-8s %-16s %s%s", item.ID, priority, status, item.DueLabel(), item.Title, tags)
}

func heading(value string, color bool) string {
	value = strings.ToUpper(value)
	if !color {
		return value
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render(value)
}

func section(value string, color bool) string {
	if !color {
		return value
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")).Render(value)
}
