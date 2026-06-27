# just-a-todo

A local terminal todo app written in Go. It has a full-screen TUI for daily management and fast print-and-exit commands for shell startup.

Building from source requires Go 1.26.3 or newer.

## Install

From GitHub:

```sh
go install github.com/DiLRandI/just-a-todo/cmd/todo@latest
```

From a local checkout:

```sh
make install
```

Make sure Go's install directory is on your `PATH`. By default that is usually `~/go/bin`.

## Usage

Open the TUI:

```sh
todo
todo tui
```

Create todos from the terminal:

```sh
todo add "Pay rent" --due "tomorrow 09:00" --priority high --tag home --repeat monthly
todo add "Read Go release notes" --tag learning
```

Print reminders and exit:

```sh
todo today
todo tomorrow
todo week
todo month
```

These commands show overdue open todos plus todos in the requested calendar range. `week` uses a Monday-start calendar week, and `month` uses the current calendar month.

The TUI's `today`, `tomorrow`, `this week`, and `this month` views also include earlier overdue open todos. Future-period views only include their exact calendar range.

Manage todos:

```sh
todo list
todo list --tag home --priority high
todo list --search "release notes"
todo list --done
todo list --archived
todo list --all
todo done 12
todo reopen 12
todo edit 12 --due "2026-05-13 17:00" --priority normal
todo archive 12
todo unarchive 12
todo remove 12 --force
```

For shell startup reminders, add this to your shell profile:

```sh
todo today --no-color
```

`--no-color` and the standard `NO_COLOR` environment variable disable styled color output in both print commands and the TUI.

## TUI Keys

- `j/k` or arrow keys: move
- `1`: open todos
- `2`: today
- `3`: tomorrow
- `4`: this week
- `5`: next week
- `6`: this month
- `7`: next month
- `8`: no due date
- `9`: completed
- `0`: archived
- `n`: create
- `e`: edit
- `enter`, `space`, or `d`: mark done or reopen
- `a`: archive
- `u`: unarchive
- `x`: remove with confirmation
- `/`: search
- `r`: refresh
- `q`: quit

When adding or editing a todo, the `Due` and `Repeat` fields show suggestions. Use `up/down` to choose one, or keep typing to filter the list. The `Notes` field is multiline; use `ctrl+s` to save from Notes.

## Dates and Recurrence

Accepted due values:

- `today`
- `tomorrow`
- `today 09:00`
- `tomorrow 17:30`
- `2026-05-13`
- `2026-05-13 17:00`

Date-only todos are all-day todos. They become overdue only after that date has passed. Timed todos become overdue after their local date and time.

Repeat rules are `none`, `daily`, `weekly`, and `monthly`. Completing a recurring todo keeps the completed occurrence and creates the next open occurrence. Monthly todos retain their intended day across short months: a January 31 series continues on February 28 and then March 31.

A completed recurring occurrence cannot be reopened while its generated successor still exists, because that would create two active occurrences in the same series. Remove the successor first if you intentionally need to reopen the earlier occurrence.

## Data Location

The SQLite database is stored at:

```text
$XDG_DATA_HOME/todo/todo.db
```

If `XDG_DATA_HOME` is not set, it falls back to:

```text
~/.local/share/todo/todo.db
```

Override it per command with:

```sh
TODO_DB_PATH=/tmp/todo.db todo today
todo --db /tmp/todo.db list
```

Schema migrations run automatically when the app opens the database.

### Backup and restore

Close the TUI before restoring a database. For a consistent backup while other commands may be running, use SQLite's backup command:

```sh
sqlite3 ~/.local/share/todo/todo.db ".backup todo-backup.db"
```

To restore, close all `todo` processes, preserve the current database as a fallback, and replace it:

```sh
mv ~/.local/share/todo/todo.db ~/.local/share/todo/todo.db.old
cp todo-backup.db ~/.local/share/todo/todo.db
rm -f ~/.local/share/todo/todo.db-shm ~/.local/share/todo/todo.db-wal
```

Adjust the paths if you use `XDG_DATA_HOME`, `TODO_DB_PATH`, or `--db`.

## Development

```sh
make build
make test
make check
make fmt
make tidy
make run
```

`make test` runs the unit and integration tests. `make check` also checks formatting and runs `go vet`. CI runs the same checks plus the race detector. See [CONTRIBUTING.md](CONTRIBUTING.md) for the complete validation and release process.

## License

No license has been selected for this repository. Until the owner adds one, the source is available for viewing but no permission to copy, modify, or redistribute it is granted.
