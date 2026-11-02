package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogin(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCall string
		wantErr  error
	}{
		{name: "claude", args: []string{"login", "claude"}, wantCall: "login claude"},
		{name: "codex", args: []string{"login", "codex"}, wantCall: "login codex"},
		{name: "github", args: []string{"login", "github"}, wantCall: "login github"},
		{name: "unknown provider", args: []string{"login", "bogus"}, wantErr: errAny},
		{name: "no provider", args: []string{"login"}, wantErr: errAny},
		{name: "too many arguments", args: []string{"login", "claude", "codex"}, wantErr: errAny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &fakeManager{}
			_, err := execute(t, testRoot(m), tt.args...)
			if tt.wantErr != nil {
				requireErr(t, err, tt.wantErr)
				assert.Empty(t, m.calls, "a refused command calls nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{tt.wantCall}, m.calls)
		})
	}
}
