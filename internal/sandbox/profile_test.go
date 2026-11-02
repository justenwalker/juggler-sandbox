package sandbox

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// update rewrites the golden files from the current code.
var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// goldenTools are the tool paths that the golden profiles were generated with.
func goldenTools() Tools {
	return Tools{
		"nono":    "/usr/local/bin/nono",
		"juggler": "/usr/local/bin/juggler",
		"node":    "/opt/homebrew/bin/node",
		"claude":  "/usr/local/bin/claude",
		"codex":   "/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex",
		"git":     "/usr/bin/git",
		"gh":      "/opt/homebrew/bin/gh",
	}
}

func testProfileOptions() ProfileOptions {
	l := NewLayout("/home/test/.local/share/juggler-sandbox", "/home/test/.local/state/juggler-sandbox")
	return ProfileOptions{
		Layout:  l,
		Session: l.Session(testSession),
		Home:    "/home/test",
		Tools:   goldenTools(),
		Project: "/src/app",
		Repo:    testRepo,
	}
}

func marshalProfile(t *testing.T, p Profile) string {
	t.Helper()
	data, err := json.Marshal(p)
	require.NoError(t, err)
	return string(data)
}

func TestBuildProfile(t *testing.T) {
	t.Run("matches the golden file", func(t *testing.T) {
		golden := filepath.Join("testdata", "profile-session.golden.json")
		got, err := json.MarshalIndent(BuildProfile(testProfileOptions()), "", "  ")
		require.NoError(t, err)
		if *update {
			require.NoError(t, os.WriteFile(golden, append(got, '\n'), 0o600))
		}
		assert.JSONEq(t, readFile(t, golden), string(got))
	})
	// The golden file pins the exact values. These subtests state properties.
	t.Run("inherits default, then each extra profile in order", func(t *testing.T) {
		opt := testProfileOptions()
		assert.Equal(t, []string{"default"}, BuildProfile(opt).Extends)
		opt.Extends = []string{"go-dev", "mine"}
		assert.Equal(t, []string{"default", "go-dev", "mine"}, BuildProfile(opt).Extends)
	})
	t.Run("does not share the extends slice with the caller", func(t *testing.T) {
		opt := testProfileOptions()
		opt.Extends = make([]string, 1, 4)
		opt.Extends[0] = "go-dev"
		_ = BuildProfile(opt)
		assert.Equal(t, []string{"go-dev"}, opt.Extends)
	})
	t.Run("keeps the Keychain denied", func(t *testing.T) {
		p := BuildProfile(testProfileOptions())
		assert.Contains(t, p.Groups.Include, "deny_keychains_macos")
		assert.Contains(t, p.Filesystem.Deny, "/home/test/Library/Keychains")
		assert.Contains(t, p.Filesystem.Deny, testProfileOptions().Session.Keychains())
		for _, path := range p.Filesystem.BypassProtection {
			assert.NotContains(t, path, "Keychains")
		}
		assert.NotContains(t, p.Network.AllowDomain, "*")
	})
	t.Run("grants the project, the session data, and the shared identity", func(t *testing.T) {
		opt := testProfileOptions()
		p := BuildProfile(opt)
		assert.Equal(t, []string{
			"/src/app", opt.Session.Data(), opt.Layout.ClaudeDir(), opt.Layout.CodexDir(), opt.Layout.JugglerDir(),
		}, p.Filesystem.Allow)
		assert.Equal(t, opt.Layout.JugglerDir(), p.Environment.SetVars["JUGGLER_CONFIG_DIR"])
		assert.Equal(t, opt.Layout.ClaudeDir(), p.Environment.SetVars["CLAUDE_CONFIG_DIR"])
		assert.Equal(t, opt.Layout.CodexDir(), p.Environment.SetVars["CODEX_HOME"])
		assert.Equal(t, opt.Session.Home(), p.Environment.SetVars["HOME"])
	})
	t.Run("denies the control directories and the machine server state", func(t *testing.T) {
		opt := testProfileOptions()
		p := BuildProfile(opt)
		for _, path := range []string{opt.Session.Control(), opt.Layout.IdentityControl(), opt.Layout.Machine()} {
			assert.Contains(t, p.Filesystem.Deny, path)
		}
	})
	t.Run("a login has no project", func(t *testing.T) {
		opt := testProfileOptions()
		opt.Session, opt.Project, opt.Repo = opt.Layout.Login(), "", ""
		p := BuildProfile(opt)
		assert.Equal(t, []string{
			opt.Session.Data(), opt.Layout.ClaudeDir(), opt.Layout.CodexDir(), opt.Layout.JugglerDir(),
		}, p.Filesystem.Allow)
	})
	t.Run("lets through the variables of a session child", func(t *testing.T) {
		p := BuildProfile(testProfileOptions())
		for _, key := range []string{
			envIngressSecret, envSessionToken, envSupervisorAddr, envSupervisorToken, envSupervisorSession,
		} {
			assert.Contains(t, p.Environment.AllowVars, key)
		}
		assert.Empty(t, p.Network.OpenPort, "SessionChild grants its ports per run")
	})
}

func TestReadPaths(t *testing.T) {
	tests := []struct {
		name  string
		tools Tools
		want  []string
	}{
		{name: "empty", tools: Tools{}, want: nil},
		{
			name:  "duplicate paths collapse",
			tools: Tools{"git": "/b", "gh": "/b", "node": "/a"},
			want:  []string{"/a", "/b"},
		},
		{
			name: "two tools in one bundle add one bundle path",
			tools: Tools{
				"codex":   "/Applications/X.app/Contents/bin/codex",
				"juggler": "/Applications/X.app/Contents/bin/juggler",
			},
			want: []string{
				"/Applications/X.app/Contents/bin/codex",
				"/Applications/X.app/Contents/bin/juggler",
				"/Applications/X.app",
			},
		},
		{
			name: "bundles follow the tool order",
			tools: Tools{
				"gh":    "/Applications/B.app/Contents/gh",
				"nono":  "/Applications/A.app/Contents/nono",
				"node":  "/usr/bin/node",
				"extra": "/Applications/C.app/Contents/extra",
			},
			want: []string{
				"/Applications/A.app/Contents/nono",
				"/Applications/B.app/Contents/gh",
				"/Applications/C.app/Contents/extra",
				"/usr/bin/node",
				"/Applications/A.app",
				"/Applications/B.app",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, readPaths(tt.tools))
		})
	}
}
