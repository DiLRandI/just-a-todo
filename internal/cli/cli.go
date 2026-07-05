package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DiLRandI/just-a-todo/internal/dateparse"
	"github.com/DiLRandI/just-a-todo/internal/store"
	"github.com/DiLRandI/just-a-todo/internal/todo"
	"github.com/DiLRandI/just-a-todo/internal/tui"
	"github.com/spf13/cobra"
)

var version = "dev"

type options struct {
	dbPath  string
	noColor bool
}

func SetVersion(v string) {
	version = v
}

func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	opts := &options{}
	root := &cobra.Command{
		Use:           "todo",
		Short:         "A terminal todo app",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(ctx, opts)
		},
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	root.InitDefaultVersionFlag()
	root.PersistentFlags().StringVar(&opts.dbPath, "db", "", "path to the SQLite database")
	root.PersistentFlags().BoolVar(&opts.noColor, "no-color", false, "disable ANSI color output")

	root.AddCommand(
		newTUICommand(ctx, opts),
		newAddCommand(ctx, opts, stdout),
		newListCommand(ctx, opts, stdout),
		newDateCommand(ctx, opts, stdout, "today"),
		newDateCommand(ctx, opts, stdout, "tomorrow"),
		newDateCommand(ctx, opts, stdout, "week"),
		newDateCommand(ctx, opts, stdout, "month"),
		newDoneCommand(ctx, opts, stdout),
		newReopenCommand(ctx, opts, stdout),
		newEditCommand(ctx, opts, stdout),
		newArchiveCommand(ctx, opts, stdout),
		newUnarchiveCommand(ctx, opts, stdout),
		newRemoveCommand(ctx, opts, stdout),
	)

	return root.ExecuteContext(ctx)
}

func newTUICommand(ctx context.Context, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:          "tui",
		Short:        "Open the interactive TUI",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(ctx, opts)
		},
	}
}

func newAddCommand(ctx context.Context, opts *options, stdout io.Writer) *cobra.Command {
	var dueInput, priorityInput, notes, repeatInput string
	var tags []string

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a todo",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			due, err := parseOptionalDue(dueInput)
			if err != nil {
				return err
			}
			priority, err := todo.NormalizePriority(priorityInput)
			if err != nil {
				return err
			}
			repeat, err := todo.NormalizeRepeat(repeatInput)
			if err != nil {
				return err
			}

			st, err := openStore(ctx, opts)
			if err != nil {
				return err
			}
			defer st.Close()

			item, err := st.CreateTodo(ctx, todo.CreateParams{
				Title:      strings.Join(args, " "),
				Notes:      notes,
				Priority:   priority,
				DueDate:    due.Date,
				DueTime:    due.Time,
				RepeatRule: repeat,
				Tags:       tags,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Created #%d: %s\n", item.ID, item.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&dueInput, "due", "", "due date, e.g. today, tomorrow 09:00, 2026-05-13 17:00")
	cmd.Flags().StringVar(&priorityInput, "priority", "normal", "priority: low, normal, high")
	cmd.Flags().StringVar(&notes, "notes", "", "optional notes")
	cmd.Flags().StringVar(&repeatInput, "repeat", "none", "repeat rule: none, daily, weekly, monthly")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "tag; repeat or comma-separate for multiple tags")
	return cmd
}

func newListCommand(ctx context.Context, opts *options, stdout io.Writer) *cobra.Command {
	var tag, priorityInput, search string
	var done, archived, all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List todos",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			priority, err := normalizeOptionalPriority(priorityInput)
			if err != nil {
				return err
			}
			filter := store.ListFilter{
				AllStatuses: all,
				Tag:         tag,
				Priority:    priority,
				Search:      search,
			}
			if done {
				filter.Status = todo.StatusDone
			}
			if archived {
				filter.ArchivedOnly = true
				filter.IncludeArchived = true
				if !done {
					filter.AllStatuses = true
				}
			}

			st, err := openStore(ctx, opts)
			if err != nil {
				return err
			}
			defer st.Close()

			items, err := st.ListTodos(ctx, filter)
			if err != nil {
				return err
			}
			printSimpleList(stdout, "Todos", items, opts.colorEnabled())
			return nil
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	cmd.Flags().StringVar(&priorityInput, "priority", "", "filter by priority")
	cmd.Flags().StringVar(&search, "search", "", "search title and notes")
	cmd.Flags().BoolVar(&done, "done", false, "show completed todos")
	cmd.Flags().BoolVar(&archived, "archived", false, "show archived todos")
	cmd.Flags().BoolVar(&all, "all", false, "show open and completed todos")
	cmd.MarkFlagsMutuallyExclusive("done", "all")
	return cmd
}

func newDateCommand(ctx context.Context, opts *options, stdout io.Writer, name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Print %s todos and exit", name),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			now := time.Now()
			r, err := dateparse.RangeFor(name, now)
			if err != nil {
				return err
			}

			st, err := openStore(ctx, opts)
			if err != nil {
				return err
			}
			defer st.Close()

			items, err := st.ListTodos(ctx, store.ListFilter{DueEnd: r.EndDate})
			if err != nil {
				return err
			}
			printDateSummary(stdout, r, items, now, opts.colorEnabled())
			return nil
		},
	}
}

func newDoneCommand(ctx context.Context, opts *options, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "done <id>",
		Short: "Mark a todo done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := dateparse.ParseID(args[0])
			if err != nil {
				return err
			}
			st, err := openStore(ctx, opts)
			if err != nil {
				return err
			}
			defer st.Close()

			item, next, err := st.CompleteTodo(ctx, id, time.Now())
			if err != nil {
				return friendlyStoreErr(err)
			}
			fmt.Fprintf(stdout, "Done #%d: %s\n", item.ID, item.Title)
			if next != nil {
				fmt.Fprintf(stdout, "Created next #%d due %s\n", next.ID, next.DueLabel())
			}
			return nil
		},
	}
}

func newReopenCommand(ctx context.Context, opts *options, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a completed todo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := dateparse.ParseID(args[0])
			if err != nil {
				return err
			}
			st, err := openStore(ctx, opts)
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.ReopenTodo(ctx, id); err != nil {
				return friendlyStoreErr(err)
			}
			fmt.Fprintf(stdout, "Reopened #%d\n", id)
			return nil
		},
	}
}

func newEditCommand(ctx context.Context, opts *options, stdout io.Writer) *cobra.Command {
	var title, notes, dueInput, priorityInput, repeatInput string
	var tags []string
	var clearDue, clearTags bool

	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a todo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := dateparse.ParseID(args[0])
			if err != nil {
				return err
			}

			var params todo.UpdateParams
			if cmd.Flags().Changed("title") {
				params.Title = &title
			}
			if cmd.Flags().Changed("notes") {
				params.Notes = &notes
			}
			if cmd.Flags().Changed("priority") {
				priority, err := todo.NormalizePriority(priorityInput)
				if err != nil {
					return err
				}
				params.Priority = &priority
			}
			if cmd.Flags().Changed("repeat") {
				repeat, err := todo.NormalizeRepeat(repeatInput)
				if err != nil {
					return err
				}
				params.RepeatRule = &repeat
			}
			if clearDue {
				empty := ""
				params.DueDate = &empty
				params.DueTime = &empty
			} else if cmd.Flags().Changed("due") {
				due, err := parseOptionalDue(dueInput)
				if err != nil {
					return err
				}
				params.DueDate = &due.Date
				params.DueTime = &due.Time
			}
			if clearTags {
				empty := []string{}
				params.Tags = &empty
			} else if cmd.Flags().Changed("tag") {
				normalized := todo.NormalizeTags(tags)
				params.Tags = &normalized
			}

			st, err := openStore(ctx, opts)
			if err != nil {
				return err
			}
			defer st.Close()
			item, err := st.UpdateTodo(ctx, id, params)
			if err != nil {
				return friendlyStoreErr(err)
			}
			fmt.Fprintf(stdout, "Updated #%d: %s\n", item.ID, item.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&notes, "notes", "", "new notes")
	cmd.Flags().StringVar(&dueInput, "due", "", "new due date")
	cmd.Flags().StringVar(&priorityInput, "priority", "", "new priority")
	cmd.Flags().StringVar(&repeatInput, "repeat", "", "new repeat rule")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "replace tags")
	cmd.Flags().BoolVar(&clearDue, "clear-due", false, "clear due date and time")
	cmd.Flags().BoolVar(&clearTags, "clear-tags", false, "clear tags")
	cmd.MarkFlagsMutuallyExclusive("due", "clear-due")
	cmd.MarkFlagsMutuallyExclusive("tag", "clear-tags")
	return cmd
}

func newArchiveCommand(ctx context.Context, opts *options, stdout io.Writer) *cobra.Command {
	return stateCommand(ctx, opts, stdout, "archive", "Archive a todo", func(st *store.Store, id int64) error {
		return st.ArchiveTodo(ctx, id)
	})
}

func newUnarchiveCommand(ctx context.Context, opts *options, stdout io.Writer) *cobra.Command {
	return stateCommand(ctx, opts, stdout, "unarchive", "Unarchive a todo", func(st *store.Store, id int64) error {
		return st.UnarchiveTodo(ctx, id)
	})
}

func newRemoveCommand(ctx context.Context, opts *options, stdout io.Writer) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Permanently remove a todo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				return fmt.Errorf("remove is permanent; rerun with --force")
			}
			id, err := dateparse.ParseID(args[0])
			if err != nil {
				return err
			}
			st, err := openStore(ctx, opts)
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.DeleteTodo(ctx, id); err != nil {
				return friendlyStoreErr(err)
			}
			fmt.Fprintf(stdout, "Removed #%d\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm permanent deletion")
	return cmd
}

func stateCommand(ctx context.Context, opts *options, stdout io.Writer, use, short string, fn func(*store.Store, int64) error) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := dateparse.ParseID(args[0])
			if err != nil {
				return err
			}
			st, err := openStore(ctx, opts)
			if err != nil {
				return err
			}
			defer st.Close()
			if err := fn(st, id); err != nil {
				return friendlyStoreErr(err)
			}
			fmt.Fprintf(stdout, "%s #%d\n", commandPastTense(use), id)
			return nil
		},
	}
}

func runTUI(ctx context.Context, opts *options) error {
	st, err := openStore(ctx, opts)
	if err != nil {
		return err
	}
	defer st.Close()
	return tui.Run(ctx, st, opts.colorEnabled())
}

func openStore(ctx context.Context, opts *options) (*store.Store, error) {
	st, err := store.Open(opts.dbPath)
	if err != nil {
		return nil, err
	}
	if err := st.Migrate(ctx); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

func parseOptionalDue(input string) (dateparse.Due, error) {
	if strings.TrimSpace(input) == "" {
		return dateparse.Due{}, nil
	}
	return dateparse.ParseDue(input, time.Now())
}

func normalizeOptionalPriority(input string) (todo.Priority, error) {
	if strings.TrimSpace(input) == "" {
		return "", nil
	}
	return todo.NormalizePriority(input)
}

func friendlyStoreErr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("todo not found")
	}
	if errors.Is(err, store.ErrRecurringSuccessorExists) {
		return fmt.Errorf("cannot reopen recurring todo: its next occurrence already exists")
	}
	return err
}

func (o options) colorEnabled() bool {
	return !o.noColor && strings.TrimSpace(os.Getenv("NO_COLOR")) == ""
}

func commandPastTense(command string) string {
	switch command {
	case "archive":
		return "Archived"
	case "unarchive":
		return "Unarchived"
	default:
		return strings.TrimSpace(command)
	}
}
