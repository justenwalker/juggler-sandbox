package sandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidSessionID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "machine server id", in: "06ef2310", want: true},
		{name: "long hex", in: "0123456789abcdef0123456789abcdef", want: true},
		{name: "too short", in: "06ef231"},
		{name: "uppercase", in: "06EF2310"},
		{name: "parent directory", in: "../06ef2310"},
		{name: "slash", in: "06ef/2310"},
		{name: "empty", in: ""},
		{name: "login", in: "login"},
		{name: "trailing newline", in: "06ef2310\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, validSessionID(tt.in))
		})
	}
}

func TestValidRepo(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "owner and repo", in: "plotly/dash-enterprise", want: true},
		{name: "dots and underscores", in: "a_b/c.d", want: true},
		{name: "url", in: "https://github.com/a/b"},
		{name: "relative", in: "../repo"},
		{name: "dotdot repo", in: "owner/.."},
		{name: "dot owner", in: "./repo"},
		{name: "shell metacharacter", in: "a/b;echo"},
		{name: "three parts", in: "a/b/c"},
		{name: "empty", in: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, validRepo(tt.in))
		})
	}
}

func TestValidateExtends(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		wantErr error
	}{
		{name: "none", in: nil},
		{name: "built-in", in: []string{"go-dev"}},
		{name: "several", in: []string{"go-dev", "my_profile.v2"}},
		{name: "empty", in: []string{""}, wantErr: ErrInvalidProfile},
		{name: "path", in: []string{"/tmp/p.json"}, wantErr: ErrInvalidProfile},
		{name: "relative path", in: []string{"../p"}, wantErr: ErrInvalidProfile},
		{name: "space", in: []string{"a b"}, wantErr: ErrInvalidProfile},
		{name: "one bad among good", in: []string{"go-dev", "x/y"}, wantErr: ErrInvalidProfile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExtends(tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		in      int
		wantErr error
	}{
		{name: "lowest", in: 1024},
		{name: "default", in: 3939},
		{name: "highest", in: 65535},
		{name: "below range", in: 1023, wantErr: ErrInvalidPort},
		{name: "above range", in: 65536, wantErr: ErrInvalidPort},
		{name: "zero", in: 0, wantErr: ErrInvalidPort},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
