package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tlmysten/worktree-tools/internal/cli"
	"github.com/tlmysten/worktree-tools/internal/proxy"
	"github.com/tlmysten/worktree-tools/internal/ui"
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

	fmt.Fprintf(os.Stderr, "%s %v\n", ui.Tag(os.Stderr, "ERR"), err)
	os.Exit(1)
}
