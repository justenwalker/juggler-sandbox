package sandbox

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareSandbox(t *testing.T) {
	t.Run("writes and validates the profile", func(t *testing.T) {
		h := newHarness(t)
		sp := h.layout().Session(testSession)
		s, err := h.m.prepareSandbox(t.Context(), h.layout(), sandboxRequest{
			paths: sp, tools: h.tools, project: h.project, repo: testRepo, action: "session",
		})
		require.NoError(t, err)
		assert.FileExists(t, sp.Profile())
		require.Len(t, h.runner.runs, 1)
		assert.Equal(t, []string{h.tools["nono"], "profile", "validate", sp.Profile()}, h.runner.runs[0].Argv)
		assert.Equal(t, hostEnvironment(), h.runner.runs[0].Env)
		assert.Equal(t, sp.ControlLog("session", "20261030-123456"), s.base[3])
	})
	t.Run("stops at a failed validation", func(t *testing.T) {
		h := newHarness(t)
		h.runner.runErr = failWhen("validate", os.ErrInvalid)
		_, err := h.m.prepareSandbox(t.Context(), h.layout(), sandboxRequest{
			paths: h.layout().Session(testSession), tools: h.tools, action: "session",
		})
		require.ErrorIs(t, err, os.ErrInvalid)
		require.EqualError(t, err, "sandbox: validate profile: invalid argument")
	})
}

func TestSessionCommand(t *testing.T) {
	s := session{base: []string{"/nono", "run"}, env: map[string]string{"A": "1"}}
	c := s.command("/work", "/bin/tool", "x")
	assert.Equal(t, []string{"/nono", "run", "--workdir", "/work", "--", "/bin/tool", "x"}, c.Argv)
	assert.Equal(t, "/work", c.Dir)
	assert.Equal(t, map[string]string{"A": "1"}, c.Env)
	assert.Equal(t, []string{"/nono", "run"}, s.base, "the base is unchanged")
}

func TestWarn(t *testing.T) {
	h := newHarness(t)
	h.m.warn(os.ErrInvalid)
	assert.Equal(t, "juggler-sandbox: invalid argument\n", h.errOut.String())
}
