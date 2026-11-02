package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSON(t *testing.T) {
	t.Run("writes indented JSON with mode 0600", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.json")
		require.NoError(t, writeJSON(path, map[string]any{"a": []int{1}, "b": "x"}))
		data := readFile(t, path)
		assert.JSONEq(t, `{"a":[1],"b":"x"}`, data)
		assert.Contains(t, data, "{\n  \"a\": [\n    1\n  ],")
		assert.True(t, strings.HasSuffix(data, "}\n"))
	})
	t.Run("replaces an existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.json")
		require.NoError(t, os.WriteFile(path, []byte("a much longer old value"), 0o600))
		require.NoError(t, writeJSON(path, 1))
		assert.Equal(t, "1\n", readFile(t, path))
	})
	t.Run("reports a value that cannot be encoded", func(t *testing.T) {
		require.Error(t, writeJSON(filepath.Join(t.TempDir(), "x.json"), make(chan int)))
	})
}

func TestWritePrivate(t *testing.T) {
	t.Run("creates the file with mode 0600", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secret")
		require.NoError(t, writePrivate(path, "value"))
		assert.Equal(t, "value", readFile(t, path))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})
	t.Run("truncates an existing file and resets its mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "secret")
		require.NoError(t, os.WriteFile(path, []byte("a much longer old value"), 0o644))
		require.NoError(t, os.Chmod(path, 0o644))
		require.NoError(t, writePrivate(path, "new"))
		assert.Equal(t, "new", readFile(t, path))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})
	t.Run("refuses a symbolic link", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "elsewhere")
		link := filepath.Join(dir, "secret")
		require.NoError(t, os.Symlink(target, link))
		require.Error(t, writePrivate(link, "value"))
		assert.NoFileExists(t, target)
	})
}
