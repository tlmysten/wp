package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tlmysten/worktree-tools/internal/cli"
	"github.com/tlmysten/worktree-tools/internal/proxy"
)

func main() {
	cmd := cli.NewRootCommand()
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		return
	}

	var exitErr proxy.CommandExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}

	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
