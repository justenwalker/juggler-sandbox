package sandbox

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// touchCredentials makes the fake Runner create the Claude credential file
// that a real login writes.
func (h *harness) touchCredentials(t *testing.T) {
	t.Helper()
	h.runner.onRun = func(c Command) {
		if strings.HasSuffix(c.Argv[len(c.Argv)-1], verbLogin) {
			require.NoError(t, os.WriteFile(h.layout().ClaudeCredentials(), nil, 0o600))
		}
	}
}

func TestLogin(t *testing.T) {
	t.Run("claude runs inside nono in the login sandbox", func(t *testing.T) {
		h := newHarness(t)
		h.touchCredentials(t)
		require.NoError(t, h.m.Login(t.Context(), ProviderClaude))
		require.Len(t, h.runner.runs, 2, "the validation, then the login")
		sp := h.layout().Login()
		assert.Equal(t, []string{h.tools["nono"], "profile", "validate", sp.Profile()}, h.runner.runs[0].Argv)
		run := h.runner.runs[1]
		want := []string{
			h.tools["nono"], "run", "--silent", "--log-file", sp.ControlLog("login", "20261030-123456"),
			"--profile", sp.Profile(), "--allow-cwd", "--no-rollback", "--strict-broker-path",
			"--workdir", sp.Data(), "--", h.tools["claude"], "auth", "login",
		}
		assert.Equal(t, want, run.Argv)
		assert.Equal(t, sp.RuntimeBin()+":"+hostPath, run.Env["PATH"])
		assert.Equal(t, rustLog, run.Env["RUST_LOG"])
		assert.Empty(t, run.Stdin, "the login reads the terminal")
		assert.Equal(t, "Open any login URL manually in your host browser.\n", h.out.String())
		assert.Contains(t, readFile(t, h.layout().CodexConfig()), `cli_auth_credentials_store = "file"`)
	})
	t.Run("codex uses device authentication and the file store", func(t *testing.T) {
		h := newHarness(t)
		require.NoError(t, h.m.Login(t.Context(), ProviderCodex))
		run := h.runner.runs[1]
		assert.Equal(t, []string{
			h.tools["codex"], "-c", `cli_auth_credentials_store="file"`, "login", "--device-auth",
		}, run.Argv[len(run.Argv)-5:])
	})
	t.Run("claude fails closed without the credential file", func(t *testing.T) {
		h := newHarness(t)
		err := h.m.Login(t.Context(), ProviderClaude)
		require.ErrorIs(t, err, ErrNoCredentialFile)
		assert.Contains(t, err.Error(), "Keychain remains denied")
	})
	t.Run("claude refuses a linked credential file", func(t *testing.T) {
		h := newHarness(t)
		h.runner.onRun = func(c Command) {
			if strings.HasSuffix(c.Argv[len(c.Argv)-1], verbLogin) {
				target := filepath.Join(h.dir, "elsewhere")
				require.NoError(t, os.WriteFile(target, nil, 0o600))
				require.NoError(t, os.Symlink(target, h.layout().ClaudeCredentials()))
			}
		}
		require.ErrorIs(t, h.m.Login(t.Context(), ProviderClaude), ErrNoCredentialFile)
	})
	t.Run("reports a failing login", func(t *testing.T) {
		h := newHarness(t)
		h.runner.runErr = failWhen("auth", os.ErrInvalid)
		require.ErrorIs(t, h.m.Login(t.Context(), ProviderClaude), os.ErrInvalid)
	})
	t.Run("refuses an unknown provider before anything runs", func(t *testing.T) {
		h := newHarness(t)
		require.ErrorIs(t, h.m.Login(t.Context(), Provider("bogus")), ErrUnknownProvider)
		assert.Empty(t, h.runner.runs)
	})
	t.Run("refuses a missing tool", func(t *testing.T) {
		h := newHarness(t)
		delete(h.lookup, "claude")
		require.ErrorIs(t, h.m.Login(t.Context(), ProviderClaude), ErrMissingTool)
	})
}

func TestLoginGitHub(t *testing.T) {
	t.Run("stores the token where only nono reads it", func(t *testing.T) {
		h := newHarness(t)
		require.NoError(t, h.m.Login(t.Context(), ProviderGitHub))
		assert.Empty(t, h.runner.runs, "nothing runs in a sandbox")
		for path, want := range map[string]string{
			h.layout().GitHubToken():    "test-token",
			h.layout().GitHubGitBasic(): "x-access-token:test-token",
		} {
			assert.Equal(t, want, readFile(t, path))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		}
		assert.Equal(t, "Stored the GitHub token outside the sandboxes. Running sessions restart to use it.\n",
			h.out.String())
	})
	tests := []struct {
		name    string
		link    func(h *harness) string
		secret  func(context.Context, string) (string, error)
		wantErr error
	}{
		{name: "symlinked token file", link: func(h *harness) string { return h.layout().GitHubToken() }, wantErr: errAny},
		{name: "symlinked basic-auth file", link: func(h *harness) string { return h.layout().GitHubGitBasic() }, wantErr: errAny},
		{name: "empty token", secret: func(context.Context, string) (string, error) { return "", nil }, wantErr: ErrEmptyToken},
		{
			name:    "failing prompt",
			secret:  func(context.Context, string) (string, error) { return "", os.ErrClosed },
			wantErr: os.ErrClosed,
		},
	}
	for _, tt := range tests {
		t.Run("refuses a "+tt.name, func(t *testing.T) {
			h := newHarness(t)
			require.NoError(t, ensureIdentity(h.layout()))
			target := filepath.Join(h.dir, "elsewhere")
			if tt.link != nil {
				require.NoError(t, os.Symlink(target, tt.link(h)))
			}
			if tt.secret != nil {
				h.m.opt.ReadSecret = tt.secret
			}
			requireErr(t, h.m.Login(t.Context(), ProviderGitHub), tt.wantErr)
			assert.NoFileExists(t, target)
			if tt.link == nil {
				assert.NoFileExists(t, h.layout().GitHubToken())
			}
		})
	}
	t.Run("prompts for a dedicated token", func(t *testing.T) {
		h := newHarness(t)
		var prompt string
		h.m.opt.ReadSecret = func(_ context.Context, p string) (string, error) { prompt = p; return "t", nil }
		require.NoError(t, h.m.Login(t.Context(), ProviderGitHub))
		assert.Equal(t, "Dedicated fine-grained GitHub token: ", prompt)
	})
	t.Run("writes nothing when the context ends after the prompt", func(t *testing.T) {
		h := newHarness(t)
		ctx, cancel := context.WithCancel(t.Context())
		h.m.opt.ReadSecret = func(context.Context, string) (string, error) {
			cancel()
			return h.token, nil
		}
		require.ErrorIs(t, h.m.Login(ctx, ProviderGitHub), context.Canceled)
		assert.NoFileExists(t, h.layout().GitHubToken())
		assert.NoFileExists(t, h.layout().GitHubGitBasic())
	})
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

// pipeWith returns the read end of a pipe that holds text. The write end
// closes at once.
func pipeWith(t *testing.T, text string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestReadSecret(t *testing.T) {
	t.Run("reads a line when input is not a terminal", func(t *testing.T) {
		var out strings.Builder
		got, err := readSecret(t.Context(), pipeWith(t, "secret\nrest\n"), &out, "Prompt: ")
		require.NoError(t, err)
		assert.Equal(t, "secret\n", got)
		assert.Equal(t, "Prompt: ", out.String())
	})
	t.Run("accepts a last line without a newline", func(t *testing.T) {
		got, err := readSecret(t.Context(), pipeWith(t, "secret"), &strings.Builder{}, "")
		require.NoError(t, err)
		assert.Equal(t, "secret", got)
	})
	t.Run("reports empty input", func(t *testing.T) {
		_, err := readSecret(t.Context(), pipeWith(t, ""), &strings.Builder{}, "")
		require.ErrorIs(t, err, io.EOF)
		assert.NotContains(t, err.Error(), "sandbox:")
	})
	t.Run("reports a failing prompt", func(t *testing.T) {
		_, err := readSecret(t.Context(), pipeWith(t, ""), failingWriter{}, "P: ")
		require.Error(t, err)
	})
	t.Run("stops when the context ends", func(t *testing.T) {
		// The write end stays open, so the read blocks until the context ends.
		r, w, err := os.Pipe()
		require.NoError(t, err)
		t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		_, err = readSecret(ctx, r, &strings.Builder{}, "")
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
