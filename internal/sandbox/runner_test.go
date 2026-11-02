package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const shell = "/bin/sh"

// stdinPipe replaces os.Stdin with a pipe that holds text, and restores it
// when the test ends.
func stdinPipe(t *testing.T, text string) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	saved := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = saved
		_ = r.Close()
	})
}

func TestCommandEnviron(t *testing.T) {
	c := Command{Env: map[string]string{"B": "2", "A": "1", "C": "x=y"}}
	assert.Equal(t, []string{"A=1", "B=2", "C=x=y"}, c.environ())
	assert.Empty(t, Command{}.environ())
}

func TestExecRunnerOutput(t *testing.T) {
	t.Run("returns stdout", func(t *testing.T) {
		out, err := ExecRunner{}.Output(t.Context(), Command{Argv: []string{shell, "-c", "echo hi"}})
		require.NoError(t, err)
		assert.Equal(t, "hi\n", out)
	})
	t.Run("inherits nothing", func(t *testing.T) {
		t.Setenv("RUNNER_TEST_SECRET", "secret")
		c := Command{
			Argv: []string{shell, "-c", `echo "${RUNNER_TEST_SECRET-unset} $FOO"`},
			Env:  map[string]string{"FOO": "bar"},
		}
		out, err := ExecRunner{}.Output(t.Context(), c)
		require.NoError(t, err)
		assert.Equal(t, "unset bar\n", out)
	})
	t.Run("sends stdin and sets the directory", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		c := Command{Argv: []string{shell, "-c", "pwd; cat"}, Dir: dir, Stdin: "input"}
		out, err := ExecRunner{}.Output(t.Context(), c)
		require.NoError(t, err)
		assert.Equal(t, dir+"\ninput", out)
	})
	t.Run("connects no terminal without stdin", func(t *testing.T) {
		stdinPipe(t, "from the terminal\n")
		out, err := ExecRunner{}.Output(t.Context(), Command{Argv: []string{shell, "-c", "cat"}})
		require.NoError(t, err)
		assert.Empty(t, out)
	})
	t.Run("reports a failing command", func(t *testing.T) {
		_, err := ExecRunner{}.Output(t.Context(), Command{Argv: []string{shell, "-c", "exit 3"}})
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.Equal(t, 3, exitErr.ExitCode())
		assert.NotContains(t, err.Error(), "sandbox:")
	})
}

func TestExecRunnerRun(t *testing.T) {
	// copyStdin copies the input of the child to the file in $1.
	copyStdin := func(path string) []string { return []string{shell, "-c", `cat > "$1"`, shell, path} }
	t.Run("runs a command", func(t *testing.T) {
		require.NoError(t, ExecRunner{}.Run(t.Context(), Command{Argv: []string{shell, "-c", "exit 0"}}))
	})
	t.Run("reports a failing command", func(t *testing.T) {
		err := ExecRunner{}.Run(t.Context(), Command{Argv: []string{shell, "-c", "exit 1"}})
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.NotContains(t, err.Error(), "sandbox:")
	})
	t.Run("connects the terminal when stdin is empty", func(t *testing.T) {
		stdinPipe(t, "from the terminal\n")
		path := filepath.Join(t.TempDir(), "got")
		require.NoError(t, ExecRunner{}.Run(t.Context(), Command{Argv: copyStdin(path)}))
		assert.Equal(t, "from the terminal\n", readFile(t, path))
	})
	t.Run("sends the stdin of the command instead of the terminal", func(t *testing.T) {
		stdinPipe(t, "from the terminal\n")
		path := filepath.Join(t.TempDir(), "got")
		require.NoError(t, ExecRunner{}.Run(t.Context(), Command{Argv: copyStdin(path), Stdin: "token\n"}))
		assert.Equal(t, "token\n", readFile(t, path))
	})
	t.Run("leaves the child to the terminal when the context is canceled", func(t *testing.T) {
		const delay = 400 * time.Millisecond
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := ExecRunner{WaitDelay: delay}.Run(ctx, Command{Argv: []string{shell, "-c", "exec sleep 30"}})
		require.Error(t, err)
		// A signal from the runner would end the child after 50 ms. The child
		// lives until WaitDelay ends, and then the runner kills it.
		assert.GreaterOrEqual(t, time.Since(start), delay)
		assert.Less(t, time.Since(start), 10*time.Second)
	})
}

func TestExecRunnerExec(t *testing.T) {
	tests := []struct {
		name string
		c    Command
	}{
		{name: "missing directory", c: Command{Argv: []string{shell}, Dir: "/no/such/dir"}},
		{name: "missing program", c: Command{Argv: []string{filepath.Join(os.TempDir(), "no-such-program")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Exec returns only on failure, so a success case would replace the test binary.
			err := ExecRunner{}.Exec(tt.c)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "sandbox:")
		})
	}
}
