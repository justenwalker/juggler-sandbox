package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justenwalker/juggler-sandbox/internal/sandbox"
)

// errAny in a wantErr field means that the case needs some error but has no
// sentinel to name. Cobra words its own argument errors without a sentinel.
var errAny = errors.New("any error")

// requireErr checks err against want. It accepts any error for errAny.
func requireErr(t *testing.T, err, want error) {
	t.Helper()
	if errors.Is(want, errAny) {
		require.Error(t, err)
		return
	}
	require.ErrorIs(t, err, want)
}

// fakeManager records the call that a command makes.
type fakeManager struct {
	calls []string
	start sandbox.StartRequest
	args  []string
	err   error
}

var _ Manager = (*fakeManager)(nil)

func (f *fakeManager) Start(_ context.Context, req sandbox.StartRequest) error {
	f.calls, f.start = append(f.calls, "start"), req
	return f.err
}

func (f *fakeManager) Login(_ context.Context, p sandbox.Provider) error {
	f.calls = append(f.calls, "login "+string(p))
	return f.err
}

func (f *fakeManager) SessionChild(_ context.Context, args []string) error {
	f.calls, f.args = append(f.calls, "session-child"), args
	return f.err
}

// execute runs the command tree with args and returns what it wrote.
func execute(t *testing.T, root *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return out.String(), err
}

func testRoot(m Manager) *cobra.Command {
	return newRoot(m, "darwin", func(int) int { return 0 })
}

func TestRoot(t *testing.T) {
	t.Run("a bare call is a usage error", func(t *testing.T) {
		m := &fakeManager{}
		_, err := execute(t, testRoot(m))
		require.ErrorIs(t, err, ErrNoCommand)
		assert.Empty(t, m.calls)
	})
	t.Run("an unknown command is an error", func(t *testing.T) {
		m := &fakeManager{}
		_, err := execute(t, testRoot(m), "stop", "a")
		require.Error(t, err)
		assert.Empty(t, m.calls)
	})
	t.Run("manager errors reach the caller", func(t *testing.T) {
		m := &fakeManager{err: sandbox.ErrNonoVersion}
		_, err := execute(t, testRoot(m), "start")
		require.ErrorIs(t, err, sandbox.ErrNonoVersion)
	})
	t.Run("help lists the commands without usage noise", func(t *testing.T) {
		root := NewRootCommand(&fakeManager{})
		assert.Equal(t, "juggler-sandbox", root.Name())
		assert.True(t, root.SilenceErrors)
		assert.True(t, root.SilenceUsage)
		assert.Len(t, root.Commands(), 3, "start, login, and the hidden session-child")
		out, err := execute(t, root, "--help")
		require.NoError(t, err)
		for _, command := range []string{"login", "start"} {
			assert.Contains(t, out, command)
		}
	})
	platform := []struct {
		name      string
		goos      string
		wantErr   error
		wantUmask bool
	}{
		{name: "macOS", goos: "darwin", wantUmask: true},
		{name: "Linux", goos: "linux", wantErr: ErrPlatform},
	}
	for _, tt := range platform {
		t.Run("platform "+tt.name, func(t *testing.T) {
			m := &fakeManager{}
			var masks []int
			root := newRoot(m, tt.goos, func(mask int) int { masks = append(masks, mask); return 0 })
			_, err := execute(t, root, "login", "claude")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, m.calls)
				assert.Empty(t, masks)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []int{0o077}, masks)
		})
	}
}
