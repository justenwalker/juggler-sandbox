package sandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLayout(t *testing.T) {
	l := NewLayout("/d", "/s")
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "Data", got: l.Data(), want: "/d"},
		{name: "State", got: l.State(), want: "/s"},
		{name: "Machine", got: l.Machine(), want: "/s/machine"},
		{name: "ToolsFile", got: l.ToolsFile(), want: "/s/machine/tools.json"},
		{name: "Identity", got: l.Identity(), want: "/d/identity"},
		{name: "IdentityControl", got: l.IdentityControl(), want: "/d/identity/control"},
		{name: "GitHubToken", got: l.GitHubToken(), want: "/d/identity/control/github-token"},
		{name: "GitHubGitBasic", got: l.GitHubGitBasic(), want: "/d/identity/control/github-git-basic"},
		{name: "ClaudeDir", got: l.ClaudeDir(), want: "/d/identity/claude"},
		{name: "ClaudeCredentials", got: l.ClaudeCredentials(), want: "/d/identity/claude/.credentials.json"},
		{name: "CodexDir", got: l.CodexDir(), want: "/d/identity/codex"},
		{name: "CodexConfig", got: l.CodexConfig(), want: "/d/identity/codex/config.toml"},
		{name: "CodexAuth", got: l.CodexAuth(), want: "/d/identity/codex/auth.json"},
		{name: "JugglerDir", got: l.JugglerDir(), want: "/d/identity/juggler"},
		{name: "Login", got: l.Login().Root(), want: "/d/identity/login"},
		{name: "Sessions", got: l.Sessions(), want: "/s/sessions"},
		{name: "Session", got: l.Session("06ef2310").Root(), want: "/s/sessions/06ef2310"},
	}
	assert.Equal(t, []string{"/d", "/s"}, l.Roots())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

func TestSessionPaths(t *testing.T) {
	p := NewLayout("/d", "/s").Session("06ef2310")
	const r = "/s/sessions/06ef2310"
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "Control", got: p.Control(), want: r + "/control"},
		{name: "Profile", got: p.Profile(), want: r + "/control/profile.json"},
		{name: "ControlLogs", got: p.ControlLogs(), want: r + "/control/logs"},
		{name: "ControlLog", got: p.ControlLog("session", "20261030-123456"), want: r + "/control/logs/session-20261030-123456.log"},
		{name: "Runtime", got: p.Runtime(), want: r + "/runtime"},
		{name: "RuntimeBin", got: p.RuntimeBin(), want: r + "/runtime/bin"},
		{name: "RuntimeHooks", got: p.RuntimeHooks(), want: r + "/runtime/hooks"},
		{name: "ToolLink", got: p.ToolLink("gh"), want: r + "/runtime/bin/gh"},
		{name: "Gitconfig", got: p.Gitconfig(), want: r + "/runtime/gitconfig"},
		{name: "Data", got: p.Data(), want: r + "/data"},
		{name: "Tmp", got: p.Tmp(), want: r + "/data/tmp"},
		{name: "Logs", got: p.Logs(), want: r + "/data/logs"},
		{name: "Home", got: p.Home(), want: r + "/data/home"},
		{name: "ConfigDir", got: p.ConfigDir(), want: r + "/data/home/.config"},
		{name: "GHConfigDir", got: p.GHConfigDir(), want: r + "/data/home/.config/gh"},
		{name: "CacheDir", got: p.CacheDir(), want: r + "/data/home/.cache"},
		{name: "Keychains", got: p.Keychains(), want: r + "/data/home/Library/Keychains"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}
