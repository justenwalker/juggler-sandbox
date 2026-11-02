package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionChildArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      []string
		wantChild bool
	}{
		{name: "no arguments", args: nil, want: nil},
		{name: "a normal command", args: []string{"start", "de6"}, want: []string{"start", "de6"}},
		{
			name: "--session-child later is not a child",
			args: []string{"start", "--session-child"}, want: []string{"start", "--session-child"},
		},
		{
			name:      "a session child",
			args:      []string{"--session-child", "--port", "0"},
			want:      []string{"session-child", "--session-child", "--port", "0"},
			wantChild: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, child := SessionChildArgs(tt.args)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantChild, child)
		})
	}
}

func TestSessionChild(t *testing.T) {
	t.Run("passes every argument without parsing", func(t *testing.T) {
		m := &fakeManager{}
		raw := []string{"--session-child", "--port", "0", "--log-file", "/l", "--dev", "--project", "/p"}
		args, _ := SessionChildArgs(raw)
		_, err := execute(t, testRoot(m), args...)
		require.NoError(t, err)
		assert.Equal(t, []string{"session-child"}, m.calls)
		assert.Equal(t, raw, m.args)
	})
	t.Run("is hidden from help", func(t *testing.T) {
		out, err := execute(t, testRoot(&fakeManager{}), "--help")
		require.NoError(t, err)
		assert.NotContains(t, out, "session-child")
	})
}
