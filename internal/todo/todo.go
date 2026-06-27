package todo

import (
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusOpen Status = "open"
	StatusDone Status = "done"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

type RepeatRule string

const (
	RepeatNone    RepeatRule = "none"
	RepeatDaily   RepeatRule = "daily"
	RepeatWeekly  RepeatRule = "weekly"
	RepeatMonthly RepeatRule = "monthly"
)

type Todo struct {
	ID          int64
	Title       string
	Notes       string
	Status      Status
	Priority    Priority
	DueDate     string
	DueTime     string
	RepeatRule  RepeatRule
	Tags        []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	ArchivedAt  *time.Time
	// RecurrenceAnchorDay preserves the intended calendar day for monthly
	// recurrences after a short month clamps an occurrence.
	RecurrenceAnchorDay int
	// GeneratedFromID identifies the occurrence that generated this one.
	GeneratedFromID *int64
}

type CreateParams struct {
	Title      string
	Notes      string
	Priority   Priority
	DueDate    string
	DueTime    string
	RepeatRule RepeatRule
	Tags       []string
}

type UpdateParams struct {
	Title      *string
	Notes      *string
	Priority   *Priority
	DueDate    *string
	DueTime    *string
	RepeatRule *RepeatRule
	Tags       *[]string
}

func NormalizePriority(value string) (Priority, error) {
	switch Priority(strings.ToLower(strings.TrimSpace(value))) {
	case "", PriorityNormal:
		return PriorityNormal, nil
	case PriorityLow:
		return PriorityLow, nil
	case PriorityHigh:
		return PriorityHigh, nil
	default:
		return "", fmt.Errorf("invalid priority %q", value)
	}
}

func NormalizeRepeat(value string) (RepeatRule, error) {
	switch RepeatRule(strings.ToLower(strings.TrimSpace(value))) {
	case "", RepeatNone:
		return RepeatNone, nil
	case RepeatDaily:
		return RepeatDaily, nil
	case RepeatWeekly:
		return RepeatWeekly, nil
	case RepeatMonthly:
		return RepeatMonthly, nil
	default:
		return "", fmt.Errorf("invalid repeat rule %q", value)
	}
}

func NormalizeTags(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	tags := make([]string, 0, len(values))
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			tag := strings.ToLower(strings.TrimSpace(part))
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			tags = append(tags, tag)
		}
	}
	return tags
}

func (t Todo) DueLabel() string {
	if t.DueDate == "" {
		return "no due date"
	}
	if t.DueTime == "" {
		return t.DueDate
	}
	return t.DueDate + " " + t.DueTime
}

func (t Todo) IsArchived() bool {
	return t.ArchivedAt != nil
}

func (t Todo) IsDone() bool {
	return t.Status == StatusDone
}
