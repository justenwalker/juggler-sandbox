// Command juggler-sandbox creates and launches an isolated macOS Juggler
// workspace with nono.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/justenwalker/juggler-sandbox/internal/cmd"
	"github.com/justenwalker/juggler-sandbox/internal/sandbox"
)

const (
	exitFailure     = 1
	exitInterrupted = 130
)

// The sandbox Manager must keep satisfying the interface that the commands call.
var _ cmd.Manager = sandbox.Manager{}

func main() {
	os.Exit(run())
}

// run runs the command line and returns the exit code.
func run() int {
	// SIGTERM is not caught. The default action ends the process at once.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	// New reads the data and state directories from the environment.
	root := cmd.NewRootCommand(sandbox.New(sandbox.Options{Juggler: os.Getenv(sandbox.EnvJuggler)}))
	args, child := cmd.SessionChildArgs(os.Args[1:])
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if ctx.Err() != nil {
		return exitInterrupted
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "juggler-sandbox: %v\n", err)
		return exitCode(err, child)
	}
	return 0
}

// exitCode returns the exit code for err. A session child exits with the
// code of Juggler, so the machine server sees how its child ended.
func exitCode(err error, child bool) int {
	var exitErr *exec.ExitError
	if child && errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
		return exitErr.ExitCode()
	}
	return exitFailure
}
