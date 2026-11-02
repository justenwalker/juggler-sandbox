package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"
)

// defaultWaitDelay is the time a child has to exit after the context is
// canceled. After that time, the runner kills the child.
const defaultWaitDelay = 10 * time.Second

// Command is one child process.
type Command struct {
	// Argv is the argument list. Argv[0] is the absolute path of the program.
	// It must not be empty.
	Argv []string
	// Env is the full environment. The child inherits nothing else.
	Env map[string]string
	// Dir is the working directory. Empty means the current directory.
	Dir string
	// Stdin is the input. Empty means that [Runner.Run] connects the terminal
	// and [Runner.Output] connects no input.
	Stdin string
}

// Runner starts child processes. It is the only path from the program to a
// child process.
type Runner interface {
	// Run runs c and waits. Stdout and stderr go to the terminal.
	Run(ctx context.Context, c Command) error
	// Output runs c, waits, and returns its stdout.
	Output(ctx context.Context, c Command) (string, error)
	// Exec replaces this process with c. It returns only on failure.
	Exec(c Command) error
}

// ExecRunner is the [Runner] that starts real processes.
// The zero value is ready to use.
//
// A canceled context does not signal the child. At a terminal, the child
// receives the interrupt of the user. After WaitDelay, the runner kills it.
type ExecRunner struct {
	// WaitDelay is the time that a child has to exit after the context is
	// canceled. Zero means 10 seconds.
	WaitDelay time.Duration
}

var _ Runner = ExecRunner{}

// Run runs c and waits. When c.Stdin is empty, the child reads the terminal.
func (r ExecRunner) Run(ctx context.Context, c Command) error {
	return r.run(ctx, c, os.Stdin, os.Stdout)
}

// Output runs c, waits, and returns its stdout. Stderr goes to the terminal.
// When c.Stdin is empty, the child has no input.
func (r ExecRunner) Output(ctx context.Context, c Command) (string, error) {
	var out bytes.Buffer
	if err := r.run(ctx, c, nil, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}

// Exec changes to c.Dir and replaces this process with c.
func (ExecRunner) Exec(c Command) error {
	if c.Dir != "" {
		if err := os.Chdir(c.Dir); err != nil {
			return fmt.Errorf("exec %s: %w", c.Argv[0], err)
		}
	}
	//nolint:gosec // G204: the program and arguments are the purpose of this type.
	if err := syscall.Exec(c.Argv[0], c.Argv, c.environ()); err != nil {
		return fmt.Errorf("exec %s: %w", c.Argv[0], err)
	}
	return nil
}

// run runs c with stdout going to stdout. The child reads c.Stdin, or stdin
// when c.Stdin is empty.
func (r ExecRunner) run(ctx context.Context, c Command, stdin io.Reader, stdout io.Writer) error {
	//nolint:gosec // G204: the program and arguments are the purpose of this type.
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...)
	cmd.Env = c.environ()
	cmd.Dir = c.Dir
	// The terminal already sends SIGINT to the child. A second signal would
	// interrupt its cleanup, so a canceled context only starts WaitDelay.
	cmd.Cancel = func() error { return nil }
	cmd.WaitDelay = r.WaitDelay
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = defaultWaitDelay
	}
	cmd.Stdin = stdin
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", c.Argv[0], err)
	}
	return nil
}

// environ returns Env as a sorted KEY=VALUE list.
func (c Command) environ() []string {
	list := make([]string, 0, len(c.Env))
	for key, value := range c.Env {
		list = append(list, key+"="+value)
	}
	slices.Sort(list)
	return list
}
