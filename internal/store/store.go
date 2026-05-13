package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DiLRandI/just-a-todo/internal/dateparse"
	"github.com/DiLRandI/just-a-todo/internal/todo"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("todo not found")

type Store struct {
	db *sql.DB
}

type ListFilter struct {
	AllStatuses     bool
	Status          todo.Status
	IncludeArchived bool
	ArchivedOnly    bool
	NoDueOnly       bool
	DueStart        string
	DueEnd          string
	Tag             string
	Priority        todo.Priority
	Search          string
}

func DefaultPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("TODO_DB_PATH")); override != "" {
		return override, nil
	}
	if dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dataHome != "" {
		return filepath.Join(dataHome, "todo", "todo.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "todo", "todo.db"), nil
}

func Open(path string) (*Store, error) {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS todos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			notes TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done')),
			priority TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high')),
			due_date TEXT NULL,
			due_time TEXT NULL,
			repeat_rule TEXT NOT NULL DEFAULT 'none' CHECK (repeat_rule IN ('none', 'daily', 'weekly', 'monthly')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT NULL,
			archived_at TEXT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);`,
		`CREATE TABLE IF NOT EXISTS todo_tags (
			todo_id INTEGER NOT NULL REFERENCES todos(id) ON DELETE CASCADE,
			tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			PRIMARY KEY (todo_id, tag_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_todos_status_archived ON todos(status, archived_at);`,
		`CREATE INDEX IF NOT EXISTS idx_todos_due ON todos(due_date, due_time);`,
		`CREATE INDEX IF NOT EXISTS idx_todos_priority ON todos(priority);`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, ?);`,
	}

	now := time.Now().Format(time.RFC3339)
	for _, statement := range statements[:len(statements)-1] {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, statements[len(statements)-1], now); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) CreateTodo(ctx context.Context, params todo.CreateParams) (todo.Todo, error) {
	if err := validateCreate(params); err != nil {
		return todo.Todo{}, err
	}
	priority, _ := todo.NormalizePriority(string(params.Priority))
	repeat, _ := todo.NormalizeRepeat(string(params.RepeatRule))
	tags := todo.NormalizeTags(params.Tags)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return todo.Todo{}, err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	result, err := tx.ExecContext(
		ctx, `INSERT INTO todos (
		title, notes, status, priority, due_date, due_time, repeat_rule, created_at, updated_at
	) VALUES (?, ?, 'open', ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(params.Title),
		params.Notes,
		priority,
		nullableString(params.DueDate),
		nullableString(params.DueTime),
		repeat,
		now,
		now,
	)
	if err != nil {
		return todo.Todo{}, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return todo.Todo{}, err
	}
	if err := replaceTags(ctx, tx, id, tags); err != nil {
		return todo.Todo{}, err
	}
	if err := tx.Commit(); err != nil {
		return todo.Todo{}, err
	}

	return s.GetTodo(ctx, id)
}

func (s *Store) GetTodo(ctx context.Context, id int64) (todo.Todo, error) {
	return s.getTodo(ctx, s.db, id)
}

func (s *Store) ListTodos(ctx context.Context, filter ListFilter) ([]todo.Todo, error) {
	query := `SELECT id, title, notes, status, priority, due_date, due_time, repeat_rule, created_at, updated_at, completed_at, archived_at FROM todos`
	where, args := buildWhere(filter)
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += ` ORDER BY
		CASE WHEN due_date IS NULL THEN 1 ELSE 0 END,
		due_date ASC,
		CASE WHEN due_time IS NULL THEN 1 ELSE 0 END,
		due_time ASC,
		CASE priority WHEN 'high' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END,
		id ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []todo.Todo
	for rows.Next() {
		item, err := scanTodo(rows)
		if err != nil {
			return nil, err
		}
		todos = append(todos, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	for i := range todos {
		tags, err := loadTags(ctx, s.db, todos[i].ID)
		if err != nil {
			return nil, err
		}
		todos[i].Tags = tags
	}
	return todos, nil
}

func (s *Store) UpdateTodo(ctx context.Context, id int64, params todo.UpdateParams) (todo.Todo, error) {
	current, err := s.GetTodo(ctx, id)
	if err != nil {
		return todo.Todo{}, err
	}

	if params.Title != nil {
		current.Title = strings.TrimSpace(*params.Title)
	}
	if params.Notes != nil {
		current.Notes = *params.Notes
	}
	if params.Priority != nil {
		current.Priority = *params.Priority
	}
	if params.DueDate != nil {
		current.DueDate = *params.DueDate
	}
	if params.DueTime != nil {
		current.DueTime = *params.DueTime
	}
	if params.RepeatRule != nil {
		current.RepeatRule = *params.RepeatRule
	}
	if params.Tags != nil {
		current.Tags = *params.Tags
	}

	if err := validateCreate(todo.CreateParams{
		Title:      current.Title,
		Notes:      current.Notes,
		Priority:   current.Priority,
		DueDate:    current.DueDate,
		DueTime:    current.DueTime,
		RepeatRule: current.RepeatRule,
		Tags:       current.Tags,
	}); err != nil {
		return todo.Todo{}, err
	}
	priority, _ := todo.NormalizePriority(string(current.Priority))
	repeat, _ := todo.NormalizeRepeat(string(current.RepeatRule))
	current.Priority = priority
	current.RepeatRule = repeat
	current.Tags = todo.NormalizeTags(current.Tags)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return todo.Todo{}, err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	result, err := tx.ExecContext(
		ctx, `UPDATE todos SET
		title = ?, notes = ?, priority = ?, due_date = ?, due_time = ?, repeat_rule = ?, updated_at = ?
		WHERE id = ?`,
		current.Title,
		current.Notes,
		current.Priority,
		nullableString(current.DueDate),
		nullableString(current.DueTime),
		current.RepeatRule,
		now,
		id,
	)
	if err != nil {
		return todo.Todo{}, err
	}
	if err := requireRows(result); err != nil {
		return todo.Todo{}, err
	}
	if params.Tags != nil {
		if err := replaceTags(ctx, tx, id, current.Tags); err != nil {
			return todo.Todo{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return todo.Todo{}, err
	}

	return s.GetTodo(ctx, id)
}

func (s *Store) CompleteTodo(ctx context.Context, id int64, now time.Time) (todo.Todo, *todo.Todo, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return todo.Todo{}, nil, err
	}
	defer tx.Rollback()

	item, err := s.getTodo(ctx, tx, id)
	if err != nil {
		return todo.Todo{}, nil, err
	}
	if item.Status == todo.StatusDone {
		return item, nil, tx.Commit()
	}

	stamp := now.Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `UPDATE todos SET status = 'done', completed_at = ?, updated_at = ? WHERE id = ?`, stamp, stamp, id)
	if err != nil {
		return todo.Todo{}, nil, err
	}
	if err := requireRows(result); err != nil {
		return todo.Todo{}, nil, err
	}
	item.Status = todo.StatusDone
	completedAt := now
	item.CompletedAt = &completedAt
	item.UpdatedAt = now

	var next *todo.Todo
	if item.RepeatRule != todo.RepeatNone && item.DueDate != "" {
		nextDate, err := dateparse.NextFutureDueDate(item.DueDate, item.DueTime, string(item.RepeatRule), now)
		if err != nil {
			return todo.Todo{}, nil, err
		}
		result, err := tx.ExecContext(
			ctx, `INSERT INTO todos (
			title, notes, status, priority, due_date, due_time, repeat_rule, created_at, updated_at
		) VALUES (?, ?, 'open', ?, ?, ?, ?, ?, ?)`,
			item.Title,
			item.Notes,
			item.Priority,
			nextDate,
			nullableString(item.DueTime),
			item.RepeatRule,
			stamp,
			stamp,
		)
		if err != nil {
			return todo.Todo{}, nil, err
		}
		nextID, err := result.LastInsertId()
		if err != nil {
			return todo.Todo{}, nil, err
		}
		if err := replaceTags(ctx, tx, nextID, item.Tags); err != nil {
			return todo.Todo{}, nil, err
		}
		created := todo.Todo{
			ID:         nextID,
			Title:      item.Title,
			Notes:      item.Notes,
			Status:     todo.StatusOpen,
			Priority:   item.Priority,
			DueDate:    nextDate,
			DueTime:    item.DueTime,
			RepeatRule: item.RepeatRule,
			Tags:       append([]string(nil), item.Tags...),
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		next = &created
	}

	if err := tx.Commit(); err != nil {
		return todo.Todo{}, nil, err
	}
	return item, next, nil
}

func (s *Store) ReopenTodo(ctx context.Context, id int64) error {
	now := time.Now().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE todos SET status = 'open', completed_at = NULL, updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	return requireRows(result)
}

func (s *Store) ArchiveTodo(ctx context.Context, id int64) error {
	now := time.Now().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE todos SET archived_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	if err != nil {
		return err
	}
	return requireRows(result)
}

func (s *Store) UnarchiveTodo(ctx context.Context, id int64) error {
	now := time.Now().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE todos SET archived_at = NULL, updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	return requireRows(result)
}

func (s *Store) DeleteTodo(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM todos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireRows(result)
}

type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *Store) getTodo(ctx context.Context, q querier, id int64) (todo.Todo, error) {
	row := q.QueryRowContext(ctx, `SELECT id, title, notes, status, priority, due_date, due_time, repeat_rule, created_at, updated_at, completed_at, archived_at FROM todos WHERE id = ?`, id)
	item, err := scanTodo(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return todo.Todo{}, ErrNotFound
		}
		return todo.Todo{}, err
	}
	item.Tags, err = loadTags(ctx, q, item.ID)
	if err != nil {
		return todo.Todo{}, err
	}
	return item, nil
}

func buildWhere(filter ListFilter) ([]string, []any) {
	var where []string
	var args []any

	switch {
	case filter.ArchivedOnly:
		where = append(where, "archived_at IS NOT NULL")
	case !filter.IncludeArchived:
		where = append(where, "archived_at IS NULL")
	}

	if !filter.AllStatuses {
		status := filter.Status
		if status == "" {
			status = todo.StatusOpen
		}
		where = append(where, "status = ?")
		args = append(args, status)
	}

	if filter.NoDueOnly {
		where = append(where, "due_date IS NULL")
	} else {
		if filter.DueStart != "" {
			where = append(where, "due_date >= ?")
			args = append(args, filter.DueStart)
		}
		if filter.DueEnd != "" {
			where = append(where, "due_date <= ?")
			args = append(args, filter.DueEnd)
		}
	}

	if filter.Tag != "" {
		where = append(where, `EXISTS (
			SELECT 1 FROM todo_tags tt
			JOIN tags t ON t.id = tt.tag_id
			WHERE tt.todo_id = todos.id AND t.name = ?
		)`)
		args = append(args, strings.ToLower(strings.TrimSpace(filter.Tag)))
	}

	if filter.Priority != "" {
		where = append(where, "priority = ?")
		args = append(args, filter.Priority)
	}

	if filter.Search != "" {
		needle := "%" + strings.ToLower(strings.TrimSpace(filter.Search)) + "%"
		where = append(where, "(LOWER(title) LIKE ? OR LOWER(notes) LIKE ?)")
		args = append(args, needle, needle)
	}

	return where, args
}

type scanner interface {
	Scan(...any) error
}

func scanTodo(row scanner) (todo.Todo, error) {
	var item todo.Todo
	var dueDate, dueTime, completedAt, archivedAt sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(
		&item.ID,
		&item.Title,
		&item.Notes,
		&item.Status,
		&item.Priority,
		&dueDate,
		&dueTime,
		&item.RepeatRule,
		&createdAt,
		&updatedAt,
		&completedAt,
		&archivedAt,
	)
	if err != nil {
		return todo.Todo{}, err
	}

	item.DueDate = dueDate.String
	item.DueTime = dueTime.String
	item.CreatedAt = parseStoredTime(createdAt)
	item.UpdatedAt = parseStoredTime(updatedAt)
	if completedAt.Valid {
		t := parseStoredTime(completedAt.String)
		item.CompletedAt = &t
	}
	if archivedAt.Valid {
		t := parseStoredTime(archivedAt.String)
		item.ArchivedAt = &t
	}
	return item, nil
}

func loadTags(ctx context.Context, q querier, todoID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT t.name FROM tags t JOIN todo_tags tt ON tt.tag_id = t.id WHERE tt.todo_id = ? ORDER BY t.name`, todoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func replaceTags(ctx context.Context, tx *sql.Tx, todoID int64, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM todo_tags WHERE todo_id = ?`, todoID); err != nil {
		return err
	}

	for _, tag := range todo.NormalizeTags(tags) {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name) VALUES (?)`, tag); err != nil {
			return err
		}
		var tagID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO todo_tags(todo_id, tag_id) VALUES (?, ?)`, todoID, tagID); err != nil {
			return err
		}
	}

	return nil
}

func validateCreate(params todo.CreateParams) error {
	if strings.TrimSpace(params.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if _, err := todo.NormalizePriority(string(params.Priority)); err != nil {
		return err
	}
	repeat, err := todo.NormalizeRepeat(string(params.RepeatRule))
	if err != nil {
		return err
	}
	if params.DueDate == "" && params.DueTime != "" {
		return fmt.Errorf("due time requires a due date")
	}
	if repeat != todo.RepeatNone && params.DueDate == "" {
		return fmt.Errorf("repeat requires a due date")
	}
	return nil
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func parseStoredTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func requireRows(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
