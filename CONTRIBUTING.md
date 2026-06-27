# Contributing

## Requirements

- Go 1.26.3 or newer
- SQLite CLI only if you want to exercise the documented backup procedure

## Local validation

Run these checks before submitting a change:

```sh
gofmt -w cmd internal
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

Changes to storage behavior should include migration coverage and tests against both a new database and an existing schema where applicable. Changes to recurrence should test multiple consecutive occurrences. User-visible CLI or TUI changes should update `README.md`.

## Releases

Releases are built from tags matching `v*`. Before tagging:

1. Ensure CI passes on the target commit.
2. Update user-facing documentation.
3. Create an annotated semantic-version tag, for example `v1.2.0`.
4. Push the tag.

The release workflow builds Linux, macOS, and Windows archives and publishes checksums with the GitHub release.

## License

The repository currently has no open-source license. Contributions should not be accepted until the owner selects a license and documents the terms under which contributions are made.
