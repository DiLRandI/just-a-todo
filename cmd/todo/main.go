package main

import (
	"context"
	"fmt"
	"os"

	"github.com/DiLRandI/just-a-todo/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
