package dateparse

import (
	"testing"
	"time"
)

func TestParseDue(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 30, 0, 0, time.Local)

	tests := []struct {
		name  string
		input string
		date  string
		clock string
	}{
		{name: "today", input: "today", date: "2026-05-13"},
		{name: "tomorrow with time", input: "tomorrow 09:00", date: "2026-05-14", clock: "09:00"},
		{name: "date", input: "2026-06-01", date: "2026-06-01"},
		{name: "date with time", input: "2026-06-01 17:45", date: "2026-06-01", clock: "17:45"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			due, err := ParseDue(tt.input, now)
			if err != nil {
				t.Fatal(err)
			}
			if due.Date != tt.date || due.Time != tt.clock {
				t.Fatalf("got %q %q, want %q %q", due.Date, due.Time, tt.date, tt.clock)
			}
		})
	}
}

func TestRangeForCalendarWeekAndMonth(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 30, 0, 0, time.Local)

	tests := []struct {
		name  string
		start string
		end   string
	}{
		{name: "today", start: "2026-05-13", end: "2026-05-13"},
		{name: "tomorrow", start: "2026-05-14", end: "2026-05-14"},
		{name: "week", start: "2026-05-11", end: "2026-05-17"},
		{name: "this week", start: "2026-05-11", end: "2026-05-17"},
		{name: "next week", start: "2026-05-18", end: "2026-05-24"},
		{name: "month", start: "2026-05-01", end: "2026-05-31"},
		{name: "this month", start: "2026-05-01", end: "2026-05-31"},
		{name: "next month", start: "2026-06-01", end: "2026-06-30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := RangeFor(tt.name, now)
			if err != nil {
				t.Fatal(err)
			}
			if r.StartDate != tt.start || r.EndDate != tt.end {
				t.Fatalf("%s = %s..%s, want %s..%s", tt.name, r.StartDate, r.EndDate, tt.start, tt.end)
			}
		})
	}
}

func TestIsOverdueAllDayAndTimed(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 30, 0, 0, time.Local)

	if IsOverdue("2026-05-13", "", now) {
		t.Fatal("all-day todo should not be overdue during its due date")
	}
	if !IsOverdue("2026-05-12", "", now) {
		t.Fatal("past all-day todo should be overdue")
	}
	if !IsOverdue("2026-05-13", "09:00", now) {
		t.Fatal("past timed todo should be overdue")
	}
	if IsOverdue("2026-05-13", "11:00", now) {
		t.Fatal("future timed todo should not be overdue")
	}
}

func TestNextFutureDueDate(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 30, 0, 0, time.Local)

	next, err := NextFutureDueDate("2026-05-01", "", "weekly", now)
	if err != nil {
		t.Fatal(err)
	}
	if next != "2026-05-15" {
		t.Fatalf("next = %s", next)
	}

	next, err = NextFutureDueDate("2026-01-31", "", "monthly", time.Date(2026, 2, 20, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if next != "2026-02-28" {
		t.Fatalf("monthly clamp = %s", next)
	}
}

func TestNextFutureDueDatePreservesMonthlyAnchor(t *testing.T) {
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, time.Local)
	next, err := NextFutureDueDateAnchored("2026-02-28", "", "monthly", 31, now)
	if err != nil {
		t.Fatal(err)
	}
	if next != "2026-03-31" {
		t.Fatalf("next = %s, want 2026-03-31", next)
	}
}
