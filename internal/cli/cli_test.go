package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIAddAndToday(t *testing.T) {
	db := filepath.Join(t.TempDir(), "todo.db")
	ctx := context.Background()
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := Execute(ctx, []string{"--db", db, "add", "Pay rent", "--due", "today", "--priority", "high", "--tag", "home", "--no-color"}, &out, &errOut)
	if err != nil {
		t.Fatalf("add failed: %v stderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Created #1") {
		t.Fatalf("add output = %q", out.String())
	}

	out.Reset()
	errOut.Reset()
	err = Execute(ctx, []string{"--db", db, "--no-color", "today"}, &out, &errOut)
	if err != nil {
		t.Fatalf("today failed: %v stderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "TODAY") || !strings.Contains(out.String(), "Pay rent") {
		t.Fatalf("today output = %q", out.String())
	}
}

func TestCLIRemoveRequiresForce(t *testing.T) {
	db := filepath.Join(t.TempDir(), "todo.db")
	ctx := context.Background()
	var out bytes.Buffer
	var errOut bytes.Buffer

	if err := Execute(ctx, []string{"--db", db, "add", "Delete me"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	err := Execute(ctx, []string{"--db", db, "remove", "1"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected remove without --force to fail")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("remove error = %v", err)
	}
}

func TestCLIVersionFlag(t *testing.T) {
	originalVersion := version
	SetVersion("v0.3.1")
	t.Cleanup(func() {
		SetVersion(originalVersion)
	})

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := Execute(context.Background(), []string{"--version"}, &out, &errOut)
	if err != nil {
		t.Fatalf("version failed: %v stderr=%s", err, errOut.String())
	}
	if strings.TrimSpace(out.String()) != "todo v0.3.1" {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestCLILeavesErrorRenderingToCaller(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	err := Execute(context.Background(), []string{"list", "unexpected"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected argument error")
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr should not duplicate returned error: %q", errOut.String())
	}
}

func TestCLIRejectsConflictingEditFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	err := Execute(context.Background(), []string{"edit", "1", "--due", "today", "--clear-due"}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "flags in the group") {
		t.Fatalf("error = %v", err)
	}
}
