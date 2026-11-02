package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureIdentity(t *testing.T) {
	t.Run("creates the identity with a Codex file store", func(t *testing.T) {
		l := NewLayout(t.TempDir(), t.TempDir())
		require.NoError(t, ensureIdentity(l))
		for _, dir := range []string{l.Identity(), l.IdentityControl(), l.ClaudeDir(), l.CodexDir(), l.JugglerDir()} {
			info, err := os.Stat(dir)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
		}
		assert.Equal(t, codexConfig, readFile(t, l.CodexConfig()))
	})
	t.Run("keeps an existing Codex configuration", func(t *testing.T) {
		l := NewLayout(t.TempDir(), t.TempDir())
		require.NoError(t, ensureIdentity(l))
		require.NoError(t, os.WriteFile(l.CodexConfig(), []byte("mine"), 0o600))
		require.NoError(t, ensureIdentity(l))
		assert.Equal(t, "mine", readFile(t, l.CodexConfig()))
	})
	t.Run("refuses a redirected identity directory", func(t *testing.T) {
		base := t.TempDir()
		l := NewLayout(base, t.TempDir())
		require.NoError(t, os.Symlink(t.TempDir(), l.Identity()))
		require.ErrorIs(t, ensureIdentity(l), ErrSymlink)
	})
}

func TestEnsureSession(t *testing.T) {
	t.Run("creates the directories, tool links, and Git configuration", func(t *testing.T) {
		h := newHarness(t)
		sp := h.layout().Session(testSession)
		require.NoError(t, ensureSession(sp, h.tools))
		for _, dir := range []string{sp.Control(), sp.RuntimeHooks(), sp.Tmp(), sp.Logs(), sp.GHConfigDir()} {
			info, err := os.Stat(dir)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
		}
		for _, name := range toolNames {
			target, err := os.Readlink(sp.ToolLink(name))
			require.NoError(t, err)
			assert.Equal(t, h.tools[name], target)
		}
		want := "[credential \"https://github.com\"]\n" +
			"\thelper =\n" +
			"\thelper = " + brokeredHelper + "\n" +
			"[url \"https://github.com/\"]\n" +
			"\tinsteadOf = git@github.com:\n" +
			"\tinsteadOf = ssh://git@github.com/\n" +
			"[core]\n" +
			"\thooksPath = \"" + sp.RuntimeHooks() + "\"\n"
		assert.Equal(t, want, readFile(t, sp.Gitconfig()))
	})
	t.Run("replaces the tool links with new tools", func(t *testing.T) {
		h := newHarness(t)
		sp := h.layout().Session(testSession)
		require.NoError(t, ensureSession(sp, h.tools))
		newNode := filepath.Join(h.dir, "node-2")
		h.tools["node"] = newNode
		require.NoError(t, ensureSession(sp, h.tools))
		target, err := os.Readlink(sp.ToolLink("node"))
		require.NoError(t, err)
		assert.Equal(t, newNode, target)
	})
	t.Run("refuses a redirected data directory", func(t *testing.T) {
		h := newHarness(t)
		sp := h.layout().Session(testSession)
		require.NoError(t, os.MkdirAll(sp.Root(), 0o700))
		require.NoError(t, os.Symlink(h.dir, sp.Data()))
		require.ErrorIs(t, ensureSession(sp, h.tools), ErrSymlink)
	})
}

func TestRefuseSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(dir, link))
	require.ErrorIs(t, refuseSymlink(link, "Refusing"), ErrSymlink)
	require.NoError(t, refuseSymlink(dir, "Refusing"))
	require.NoError(t, refuseSymlink(filepath.Join(dir, "missing"), "Refusing"))
}
