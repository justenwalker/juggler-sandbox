package sandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHostEnvironment(t *testing.T) {
	t.Run("drops credentials and overrides", func(t *testing.T) {
		t.Setenv("GH_TOKEN", "secret")
		t.Setenv("SSH_AUTH_SOCK", "/tmp/agent")
		t.Setenv("NONO_ALLOW", "/")
		t.Setenv("ANTHROPIC_API_KEY", "secret")
		t.Setenv("GIT_CONFIG_GLOBAL", "/evil")
		env := hostEnvironment()
		for _, name := range []string{
			"GH_TOKEN", "SSH_AUTH_SOCK", "NONO_ALLOW", "ANTHROPIC_API_KEY", "GIT_CONFIG_GLOBAL",
		} {
			assert.NotContains(t, env, name)
		}
	})
	t.Run("copies the allowed variables and sets PATH", func(t *testing.T) {
		t.Setenv("HOME", "/h")
		t.Setenv("USER", "u")
		t.Setenv("LOGNAME", "l")
		t.Setenv("LANG", "C")
		t.Setenv("TERM", "dumb")
		t.Setenv("TMPDIR", "/t")
		t.Setenv("PATH", "/evil")
		want := map[string]string{
			"HOME": "/h", "USER": "u", "LOGNAME": "l", "LANG": "C", "TERM": "dumb", "TMPDIR": "/t",
			"PATH": "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		}
		assert.Equal(t, want, hostEnvironment())
	})
}
