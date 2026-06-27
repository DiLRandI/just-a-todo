package dateparse

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Due struct {
	Date string
	Time string
}

type Range struct {
	Name      string
	Start     time.Time
	End       time.Time
	StartDate string
	EndDate   string
}

func ParseDue(input string, now time.Time) (Due, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Due{}, fmt.Errorf("due value is empty")
	}

	parts := strings.Fields(input)
	if len(parts) > 2 {
		return Due{}, fmt.Errorf("could not parse due value %q", input)
	}

	date, err := parseDateToken(parts[0], now)
	if err != nil {
		return Due{}, err
	}

	var dueTime string
	if len(parts) == 2 {
		dueTime, err = parseTimeToken(parts[1])
		if err != nil {
			return Due{}, err
		}
	}

	return Due{Date: date.Format(time.DateOnly), Time: dueTime}, nil
}

func RangeFor(name string, now time.Time) (Range, error) {
	day := startOfDay(now)
	var start, end time.Time

	switch strings.ToLower(strings.TrimSpace(name)) {
	case "today":
		start = day
		end = day
	case "tomorrow":
		start = day.AddDate(0, 0, 1)
		end = start
	case "week", "this week":
		start = startOfWeek(day)
		end = start.AddDate(0, 0, 6)
	case "next week":
		start = startOfWeek(day).AddDate(0, 0, 7)
		end = start.AddDate(0, 0, 6)
	case "month", "this month":
		start = time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
		end = start.AddDate(0, 1, -1)
	case "next month":
		thisMonth := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
		start = thisMonth.AddDate(0, 1, 0)
		end = start.AddDate(0, 1, -1)
	default:
		return Range{}, fmt.Errorf("unknown range %q", name)
	}

	return Range{
		Name:      strings.ToLower(strings.TrimSpace(name)),
		Start:     start,
		End:       end,
		StartDate: start.Format(time.DateOnly),
		EndDate:   end.Format(time.DateOnly),
	}, nil
}

func InRange(dueDate string, r Range) bool {
	return dueDate >= r.StartDate && dueDate <= r.EndDate
}

func IsOverdue(dueDate, dueTime string, now time.Time) bool {
	if dueDate == "" {
		return false
	}

	dueDay, err := time.ParseInLocation(time.DateOnly, dueDate, now.Location())
	if err != nil {
		return false
	}

	if dueTime == "" {
		return startOfDay(now).After(dueDay)
	}

	clock, err := time.ParseInLocation("15:04", dueTime, now.Location())
	if err != nil {
		return false
	}
	dueAt := time.Date(dueDay.Year(), dueDay.Month(), dueDay.Day(), clock.Hour(), clock.Minute(), 0, 0, now.Location())
	return !dueAt.After(now)
}

func NextFutureDueDate(dueDate, dueTime, rule string, now time.Time) (string, error) {
	return NextFutureDueDateAnchored(dueDate, dueTime, rule, 0, now)
}

func NextFutureDueDateAnchored(dueDate, dueTime, rule string, anchorDay int, now time.Time) (string, error) {
	base, err := time.ParseInLocation(time.DateOnly, dueDate, now.Location())
	if err != nil {
		return "", fmt.Errorf("invalid due date %q: %w", dueDate, err)
	}
	if anchorDay <= 0 {
		anchorDay = base.Day()
	}

	rule = strings.ToLower(strings.TrimSpace(rule))
	if rule == "" || rule == "none" {
		return "", fmt.Errorf("repeat rule is required")
	}

	for i := 1; i <= 3660; i++ {
		next, err := addInterval(base, rule, anchorDay, i)
		if err != nil {
			return "", err
		}
		if !IsOverdue(next.Format(time.DateOnly), dueTime, now) {
			return next.Format(time.DateOnly), nil
		}
	}

	return "", fmt.Errorf("could not calculate next due date")
}

func parseDateToken(token string, now time.Time) (time.Time, error) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "today":
		return startOfDay(now), nil
	case "tomorrow":
		return startOfDay(now).AddDate(0, 0, 1), nil
	default:
		date, err := time.ParseInLocation(time.DateOnly, token, now.Location())
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid due date %q", token)
		}
		return date, nil
	}
}

func parseTimeToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	parsed, err := time.Parse("15:04", token)
	if err != nil {
		return "", fmt.Errorf("invalid due time %q", token)
	}
	return parsed.Format("15:04"), nil
}

func addInterval(base time.Time, rule string, anchorDay, n int) (time.Time, error) {
	switch rule {
	case "daily":
		return base.AddDate(0, 0, n), nil
	case "weekly":
		return base.AddDate(0, 0, n*7), nil
	case "monthly":
		year, month := addMonths(base.Year(), base.Month(), n)
		day := min(anchorDay, lastDayOfMonth(year, month))
		return time.Date(year, month, day, 0, 0, 0, 0, base.Location()), nil
	default:
		return time.Time{}, fmt.Errorf("invalid repeat rule %q", rule)
	}
}

func addMonths(year int, month time.Month, delta int) (int, time.Month) {
	monthIndex := int(month) - 1 + delta
	year += monthIndex / 12
	monthIndex %= 12
	if monthIndex < 0 {
		monthIndex += 12
		year--
	}
	return year, time.Month(monthIndex + 1)
}

func lastDayOfMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func startOfWeek(day time.Time) time.Time {
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return day.AddDate(0, 0, -(weekday - 1))
}

func ParseID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid todo id %q", value)
	}
	return id, nil
}
