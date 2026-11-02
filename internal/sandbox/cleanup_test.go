package sandbox

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionDirs creates a sandbox directory for each id.
func (h *harness) sessionDirs(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		require.NoError(t, os.MkdirAll(h.layout().Session(id).Home(), 0o700))
	}
}

func TestCleanupSessions(t *testing.T) {
	t.Run("deletes the sandboxes of ended sessions only", func(t *testing.T) {
		h := newHarness(t)
		h.sessionDirs(t, "aaaaaaaa", "bbbbbbbb", "cccccccc")
		require.NoError(t, os.MkdirAll(filepath.Join(h.layout().Sessions(), "not-a-session"), 0o700))
		upstream, calls := fakeMachineServer(t, http.StatusOK, `[{"id":"aaaaaaaa"}]`)
		target := supervisor{Addr: strings.TrimPrefix(upstream.URL, "http://"), Token: "real-token"}
		h.m.cleanupSessions(t.Context(), h.layout(), target, "cccccccc")
		assert.DirExists(t, h.layout().Session("aaaaaaaa").Root(), "listed")
		assert.NoDirExists(t, h.layout().Session("bbbbbbbb").Root(), "ended")
		assert.DirExists(t, h.layout().Session("cccccccc").Root(), "the session that is starting")
		assert.DirExists(t, filepath.Join(h.layout().Sessions(), "not-a-session"), "not a session id")
		assert.Equal(t, []upstreamCall{{http.MethodGet, "/api/server/sessions", "", "real-token"}}, *calls)
		assert.Empty(t, h.errOut.String())
	})
	t.Run("keeps everything when the list fails", func(t *testing.T) {
		h := newHarness(t)
		h.sessionDirs(t, "bbbbbbbb")
		upstream, _ := fakeMachineServer(t, http.StatusForbidden, "no")
		target := supervisor{Addr: strings.TrimPrefix(upstream.URL, "http://"), Token: "real-token"}
		h.m.cleanupSessions(t.Context(), h.layout(), target, testSession)
		assert.DirExists(t, h.layout().Session("bbbbbbbb").Root())
		assert.Contains(t, h.errOut.String(), "juggler-sandbox: sandbox: list sessions: status 403")
	})
	t.Run("keeps everything when the answer is not a list", func(t *testing.T) {
		h := newHarness(t)
		h.sessionDirs(t, "bbbbbbbb")
		upstream, _ := fakeMachineServer(t, http.StatusOK, `{"oops":1}`)
		target := supervisor{Addr: strings.TrimPrefix(upstream.URL, "http://"), Token: "real-token"}
		h.m.cleanupSessions(t.Context(), h.layout(), target, testSession)
		assert.DirExists(t, h.layout().Session("bbbbbbbb").Root())
		assert.Contains(t, h.errOut.String(), "list sessions")
	})
	t.Run("does nothing without a machine server", func(t *testing.T) {
		h := newHarness(t)
		h.sessionDirs(t, "bbbbbbbb")
		h.m.cleanupSessions(t.Context(), h.layout(), supervisor{}, testSession)
		assert.DirExists(t, h.layout().Session("bbbbbbbb").Root())
		assert.Empty(t, h.errOut.String())
	})
	t.Run("accepts a missing sessions directory", func(t *testing.T) {
		h := newHarness(t)
		upstream, _ := fakeMachineServer(t, http.StatusOK, `[]`)
		target := supervisor{Addr: strings.TrimPrefix(upstream.URL, "http://"), Token: "real-token"}
		h.m.cleanupSessions(t.Context(), h.layout(), target, testSession)
		assert.Empty(t, h.errOut.String())
	})
}
