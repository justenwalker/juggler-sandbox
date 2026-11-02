package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justenwalker/juggler-sandbox/internal/sandbox"
)

func TestStart(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		want     sandbox.StartRequest
		wantErr  error
		wantText string
	}{
		{name: "defaults", args: []string{"start"}, want: sandbox.StartRequest{Port: 3939}},
		{
			name: "port and project",
			args: []string{"start", "--port", "3941", "/src/app"},
			want: sandbox.StartRequest{Port: 3941, Project: "/src/app"},
		},
		{
			name: "allowed hosts",
			args: []string{"start", "--allow-host", "a.ts.net", "--allow-host", "b.example"},
			want: sandbox.StartRequest{Port: 3939, AllowHosts: []string{"a.ts.net", "b.example"}},
		},
		{
			name: "extra profiles",
			args: []string{"start", "--extends", "go-dev", "--extends", "mine"},
			want: sandbox.StartRequest{Port: 3939, Extends: []string{"go-dev", "mine"}},
		},
		{name: "profile path", args: []string{"start", "--extends", "/tmp/p.json"}, wantErr: sandbox.ErrInvalidProfile},
		{
			name: "decimal port with a leading zero",
			args: []string{"start", "--port", "04000"},
			want: sandbox.StartRequest{Port: 4000},
		},
		{name: "hexadecimal port", args: []string{"start", "--port", "0x1000"}, wantErr: errAny, wantText: ErrPortText.Error()},
		{name: "octal port", args: []string{"start", "--port", "0o7777"}, wantErr: errAny, wantText: ErrPortText.Error()},
		{name: "port not a number", args: []string{"start", "--port", "x"}, wantErr: errAny, wantText: ErrPortText.Error()},
		{name: "port too low", args: []string{"start", "--port", "1023"}, wantErr: sandbox.ErrInvalidPort},
		{name: "port too high", args: []string{"start", "--port", "65536"}, wantErr: sandbox.ErrInvalidPort},
		{name: "two projects", args: []string{"start", "/a", "/b"}, wantErr: errAny, wantText: "accepts at most 1 arg(s)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &fakeManager{}
			_, err := execute(t, testRoot(m), tt.args...)
			if tt.wantErr != nil {
				requireErr(t, err, tt.wantErr)
				if tt.wantText != "" {
					require.ErrorContains(t, err, tt.wantText)
				}
				assert.Empty(t, m.calls, "a refused command calls nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{"start"}, m.calls)
			assert.Equal(t, tt.want, m.start)
		})
	}
}

func TestPortValue(t *testing.T) {
	var port int
	v := portValue{&port}
	require.NoError(t, v.Set("4000"))
	assert.Equal(t, "4000", v.String())
	assert.Equal(t, "int", v.Type())
	assert.Empty(t, portValue{}.String())
}
