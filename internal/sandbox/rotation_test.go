package sandbox

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenWatch(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, token, log string)
		want   bool
	}{
		{name: "no change", change: func(*testing.T, string, string) {}, want: false},
		{name: "a change by another nono", want: true, change: func(t *testing.T, token, _ string) {
			t.Helper()
			require.NoError(t, os.WriteFile(token, []byte("new"), 0o600))
		}},
		{name: "a change by this nono", want: false, change: func(t *testing.T, token, log string) {
			t.Helper()
			appendFile(t, log, string(captureMark)+" provider=claude_code fields=2\n")
			require.NoError(t, os.WriteFile(token, []byte("new"), 0o600))
		}},
		{name: "a removed file", want: true, change: func(t *testing.T, token, _ string) {
			t.Helper()
			require.NoError(t, os.Remove(token))
		}},
		{name: "the same contents written again", want: false, change: func(t *testing.T, token, _ string) {
			t.Helper()
			require.NoError(t, os.WriteFile(token, []byte("old"), 0o600))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			token, log := filepath.Join(dir, "token"), filepath.Join(dir, "nono.log")
			require.NoError(t, os.WriteFile(token, []byte("old"), 0o600))
			appendFile(t, log, string(captureMark)+" at login\n")
			w := newTokenWatch([]string{token, filepath.Join(dir, "absent")}, log)
			tt.change(t, token, log)
			assert.Equal(t, tt.want, w.foreign())
			assert.False(t, w.foreign(), "a change is reported once")
		})
	}
}

// appendFile appends text to the file at path.
func appendFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestEndProcessGroup(t *testing.T) {
	if syscall.Getpgrp() == os.Getpid() {
		t.Skip("the test process leads its group, so endProcessGroup would kill it")
	}
	require.ErrorIs(t, endProcessGroup(), ErrNoProcessGroup)
}
