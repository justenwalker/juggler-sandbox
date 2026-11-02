package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckProject(t *testing.T) {
	h := newHarness(t)
	inside := filepath.Join(h.state, "sessions")
	require.NoError(t, os.MkdirAll(inside, 0o700))
	require.NoError(t, os.MkdirAll(h.data, 0o700))
	parent := filepath.Join(h.home, ".local")
	link := filepath.Join(h.dir, "link")
	require.NoError(t, os.Symlink(h.project, link))
	file := filepath.Join(h.dir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	tests := []struct {
		name    string
		project string
		want    string
		wantErr error
	}{
		{name: "a project folder", project: h.project, want: h.project},
		{name: "an unclean path", project: h.project + "/./", want: h.project},
		{name: "a relative path", project: "src/app", wantErr: ErrInvalidProject},
		{name: "a missing folder", project: filepath.Join(h.dir, "missing"), wantErr: ErrInvalidProject},
		{name: "a file", project: file, wantErr: ErrInvalidProject},
		{name: "a link", project: link, wantErr: ErrInvalidProject},
		{name: "the root", project: "/", wantErr: ErrInvalidProject},
		{name: "the home", project: h.home, wantErr: ErrInvalidProject},
		{name: "the data directory", project: h.data, wantErr: ErrInvalidProject},
		{name: "inside the state directory", project: inside, wantErr: ErrInvalidProject},
		{name: "a parent of both", project: parent, wantErr: ErrInvalidProject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkProject(tt.project, h.home, h.data, h.state)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWithin(t *testing.T) {
	tests := []struct {
		name      string
		path, dir string
		want      bool
	}{
		{name: "the same path", path: "/a/b", dir: "/a/b", want: true},
		{name: "a child", path: "/a/b/c", dir: "/a/b", want: true},
		{name: "a sibling with a common prefix", path: "/a/bc", dir: "/a/b"},
		{name: "a parent", path: "/a", dir: "/a/b"},
		{name: "a name that starts with dots", path: "/a/b/..c", dir: "/a/b", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, within(tt.path, tt.dir))
		})
	}
}

func TestGitHubRepo(t *testing.T) {
	t.Run("reads the origin of a clone", func(t *testing.T) {
		h := newHarness(t)
		assert.Equal(t, testRepo, githubRepo(h.project))
	})
	t.Run("ignores a worktree, whose .git is a file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /x\n"), 0o600))
		assert.Empty(t, githubRepo(dir))
	})
	t.Run("ignores a folder without Git", func(t *testing.T) {
		assert.Empty(t, githubRepo(t.TempDir()))
	})
}

func TestOriginRepo(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{name: "https", config: "[remote \"origin\"]\n\turl = https://github.com/a/b.git\n", want: "a/b"},
		{name: "https without .git", config: "[remote \"origin\"]\nurl=https://github.com/a/b\n", want: "a/b"},
		{name: "scp-like ssh", config: "[remote \"origin\"]\n\turl = git@github.com:a/b.git\n", want: "a/b"},
		{name: "ssh url", config: "[remote \"origin\"]\n\turl = ssh://git@github.com/a/b.git\n", want: "a/b"},
		{name: "another remote", config: "[remote \"fork\"]\n\turl = https://github.com/x/y.git\n"},
		{name: "another host", config: "[remote \"origin\"]\n\turl = https://gitlab.com/a/b.git\n"},
		{name: "a path in the repo", config: "[remote \"origin\"]\n\turl = https://github.com/a/../b\n"},
		{
			name:   "origin after another section",
			config: "[core]\n\tbare = false\n[remote \"origin\"]\n\tfetch = +refs/heads/*\n\turl = https://github.com/a/b\n",
			want:   "a/b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, originRepo(strings.NewReader(tt.config)))
		})
	}
}
