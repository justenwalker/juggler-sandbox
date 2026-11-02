package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTool(t *testing.T) {
	t.Run("uses the bundle when codex is missing from PATH", func(t *testing.T) {
		h := newHarness(t)
		delete(h.lookup, "codex")
		bundled := filepath.Join(h.dir, "bundled-codex")
		require.NoError(t, os.WriteFile(bundled, nil, 0o700))
		h.m.opt.CodexBundled = bundled
		got, err := h.m.resolveTool("codex")
		require.NoError(t, err)
		assert.Equal(t, bundled, got)
	})
	t.Run("refuses a bundle without an execute bit", func(t *testing.T) {
		h := newHarness(t)
		delete(h.lookup, "codex")
		bundled := filepath.Join(h.dir, "bundled-codex")
		require.NoError(t, os.WriteFile(bundled, nil, 0o600))
		h.m.opt.CodexBundled = bundled
		_, err := h.m.resolveTool("codex")
		require.ErrorIs(t, err, ErrMissingTool)
	})
	t.Run("prefers PATH over the bundle", func(t *testing.T) {
		h := newHarness(t)
		bundled := filepath.Join(h.dir, "bundled-codex")
		require.NoError(t, os.WriteFile(bundled, nil, 0o700))
		h.m.opt.CodexBundled = bundled
		got, err := h.m.resolveTool("codex")
		require.NoError(t, err)
		assert.Equal(t, h.tools["codex"], got)
	})
	t.Run("uses the bundle for codex only", func(t *testing.T) {
		h := newHarness(t)
		delete(h.lookup, "gh")
		bundled := filepath.Join(h.dir, "bundled")
		require.NoError(t, os.WriteFile(bundled, nil, 0o700))
		h.m.opt.CodexBundled = bundled
		_, err := h.m.resolveTool("gh")
		require.ErrorIs(t, err, ErrMissingTool)
	})
	t.Run("resolves symbolic links", func(t *testing.T) {
		h := newHarness(t)
		link := filepath.Join(h.dir, "link-git")
		require.NoError(t, os.Symlink(h.tools["git"], link))
		h.lookup["git"] = link
		got, err := h.m.resolveTool("git")
		require.NoError(t, err)
		assert.Equal(t, h.tools["git"], got)
	})
	t.Run("reports a dangling link", func(t *testing.T) {
		h := newHarness(t)
		h.lookup["git"] = filepath.Join(h.dir, "dangling")
		_, err := h.m.resolveTool("git")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrMissingTool)
	})
}

func TestResolveExplicitTool(t *testing.T) {
	t.Run("resolves symbolic links", func(t *testing.T) {
		h := newHarness(t)
		dev := filepath.Join(h.dir, "dev-build")
		require.NoError(t, os.WriteFile(dev, nil, 0o700))
		link := filepath.Join(h.dir, "dev-link")
		require.NoError(t, os.Symlink(dev, link))
		got, err := resolveExplicitTool(link)
		require.NoError(t, err)
		assert.Equal(t, dev, got)
	})
	t.Run("resolves a relative path against the working directory", func(t *testing.T) {
		h := newHarness(t)
		dev := filepath.Join(h.dir, "dev-build")
		require.NoError(t, os.WriteFile(dev, nil, 0o700))
		t.Chdir(h.dir)
		got, err := resolveExplicitTool("dev-build")
		require.NoError(t, err)
		assert.Equal(t, dev, got)
	})
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) string
	}{
		{name: "missing file", setup: func(_ *testing.T, dir string) string { return filepath.Join(dir, "absent") }},
		{name: "directory", setup: func(_ *testing.T, dir string) string { return dir }},
		{name: "no execute bit", setup: func(t *testing.T, dir string) string {
			t.Helper()
			path := filepath.Join(dir, "dev-build")
			require.NoError(t, os.WriteFile(path, nil, 0o600))
			return path
		}},
	}
	for _, tt := range tests {
		t.Run("refuses a "+tt.name, func(t *testing.T) {
			_, err := resolveExplicitTool(tt.setup(t, t.TempDir()))
			require.ErrorIs(t, err, ErrMissingTool)
		})
	}
}

func TestResolveTools(t *testing.T) {
	t.Run("resolves every tool", func(t *testing.T) {
		h := newHarness(t)
		got, err := h.m.resolveTools()
		require.NoError(t, err)
		assert.Equal(t, h.tools, got)
	})
	t.Run("stops at the first missing tool", func(t *testing.T) {
		h := newHarness(t)
		delete(h.lookup, "node")
		_, err := h.m.resolveTools()
		require.ErrorIs(t, err, ErrMissingTool)
	})
}

func TestLoadTools(t *testing.T) {
	t.Run("reads what Start wrote", func(t *testing.T) {
		h := newHarness(t)
		h.writeTools(t)
		got, err := loadTools(h.layout().ToolsFile())
		require.NoError(t, err)
		assert.Equal(t, h.tools, got)
	})
	tests := []struct {
		name  string
		write func(t *testing.T, path string)
	}{
		{name: "missing file", write: func(*testing.T, string) {}},
		{name: "malformed file", write: func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
		}},
		{name: "file without node", write: func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.WriteFile(path, []byte(`{"nono":"/n"}`), 0o600))
		}},
		{name: "link", write: func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.Symlink(filepath.Join(filepath.Dir(path), "elsewhere"), path))
		}},
	}
	for _, tt := range tests {
		t.Run("refuses a "+tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tools.json")
			tt.write(t, path)
			_, err := loadTools(path)
			require.ErrorIs(t, err, ErrNoTools)
		})
	}
}

func TestJugglerPath(t *testing.T) {
	t.Run("uses the resolved juggler", func(t *testing.T) {
		h := newHarness(t)
		got, override, err := h.m.jugglerPath(h.tools)
		require.NoError(t, err)
		assert.Equal(t, h.tools["juggler"], got)
		assert.False(t, override)
	})
	t.Run("uses the override", func(t *testing.T) {
		h := newHarness(t)
		dev := filepath.Join(h.dir, "dev-juggler")
		require.NoError(t, os.WriteFile(dev, nil, 0o700))
		h.m.opt.Juggler = dev
		got, override, err := h.m.jugglerPath(h.tools)
		require.NoError(t, err)
		assert.Equal(t, dev, got)
		assert.True(t, override)
	})
	t.Run("refuses an override that is not executable", func(t *testing.T) {
		h := newHarness(t)
		h.m.opt.Juggler = filepath.Join(h.dir, "absent")
		_, _, err := h.m.jugglerPath(h.tools)
		require.ErrorIs(t, err, ErrMissingTool)
	})
}

func TestCheckNono(t *testing.T) {
	errBoom := errors.New("boom")
	tests := []struct {
		name    string
		version string
		outErr  error
		wantErr error
	}{
		{name: "pinned version", version: "nono 0.81.0\n"},
		{name: "newer version", version: "nono 0.82.0\n", wantErr: ErrNonoVersion},
		{name: "empty output", version: "", wantErr: ErrNonoVersion},
		{name: "command fails", outErr: errBoom, wantErr: errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.runner.version, h.runner.outErr = tt.version, tt.outErr
			err := h.m.checkNono(t.Context(), "/n")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, h.runner.outputs, 1)
			assert.Equal(t, []string{"/n", "--version"}, h.runner.outputs[0].Argv)
			assert.Empty(t, h.runner.runs)
		})
	}
}
