package sandbox

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// childArgs returns the arguments that a machine server gives a session
// child for project.
func childArgs(project string) []string {
	return []string{"--session-child", "--port", "0", "--log-file", "/host/logs/server.log", "--dev", "--project", project}
}

// argAfter returns the argument after the first flag in argv, or "".
func argAfter(argv []string, flag string) string {
	i := slices.Index(argv, flag)
	if i < 0 || i+1 >= len(argv) {
		return ""
	}
	return argv[i+1]
}

// openPorts returns the value of each --open-port in argv, in order.
func openPorts(argv []string) []string {
	var ports []string
	for i, arg := range argv {
		if arg == "--open-port" && i+1 < len(argv) {
			ports = append(ports, argv[i+1])
		}
	}
	return ports
}

// childReady writes tools.json and sets the environment that a machine
// server at addr gives a session child.
func (h *harness) childReady(t *testing.T, addr string) {
	t.Helper()
	h.writeTools(t)
	h.env = map[string]string{
		envIngressSecret:     "secret",
		envSessionToken:      "api-token",
		envSupervisorSession: testSession,
	}
	if addr != "" {
		h.env[envSupervisorAddr] = addr
		h.env[envSupervisorToken] = "real-token"
	}
}

// liveMachineServer is a fake machine server that lists testSession.
func liveMachineServer(t *testing.T) string {
	t.Helper()
	upstream, _ := fakeMachineServer(t, http.StatusOK, `[{"id":"`+testSession+`"}]`)
	return strings.TrimPrefix(upstream.URL, "http://")
}

func TestSessionChild(t *testing.T) {
	t.Run("creates the session sandbox and runs Juggler in it", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, liveMachineServer(t))
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		require.Len(t, h.runner.runs, 2, "the validation, then Juggler")
		sp := h.layout().Session(testSession)
		assert.Equal(t, []string{h.tools["nono"], "profile", "validate", sp.Profile()}, h.runner.runs[0].Argv)

		run := h.runner.runs[1]
		ports := openPorts(run.Argv)
		require.Len(t, ports, 2, "the proxy port and the child port")
		wantTail := []string{
			"--workdir", h.project, "--", h.tools["juggler"],
			"--session-child", "--port", ports[1], "--log-file", filepath.Join(sp.Logs(), "server.log"),
			"--dev", "--project", h.project,
		}
		assert.Equal(t, wantTail, run.Argv[len(run.Argv)-len(wantTail):])
		assert.Equal(t, []string{h.tools["nono"], "run", "--log-file", sp.ControlLog("session", "20261030-123456")}, run.Argv[:4])
		assert.Equal(t, h.project, run.Dir)

		assert.Equal(t, "secret", run.Env[envIngressSecret])
		assert.Equal(t, "api-token", run.Env[envSessionToken])
		assert.Equal(t, testSession, run.Env[envSupervisorSession])
		assert.Equal(t, "127.0.0.1:"+ports[0], run.Env[envSupervisorAddr])
		assert.Len(t, run.Env[envSupervisorToken], 64)
		for key, value := range run.Env {
			assert.NotEqual(t, "real-token", value, "the real token leaks through %s", key)
		}
		assert.Empty(t, h.out.String(), "only Juggler writes to the handshake pipe")

		var profile Profile
		require.NoError(t, json.Unmarshal([]byte(readFile(t, sp.Profile())), &profile))
		assert.Contains(t, profile.Filesystem.Allow, h.project)
		assert.Contains(t, marshalProfile(t, profile), testRepo, "GitHub credentials follow the origin remote")
		target, err := os.Readlink(sp.ToolLink("node"))
		require.NoError(t, err)
		assert.Equal(t, h.tools["node"], target)
	})
	t.Run("inherits the profiles that start named", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, liveMachineServer(t))
		h.env[EnvExtends] = "go-dev mine"
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		var profile Profile
		require.NoError(t, json.Unmarshal([]byte(readFile(t, h.layout().Session(testSession).Profile())), &profile))
		assert.Equal(t, []string{"default", "go-dev", "mine"}, profile.Extends)
	})
	t.Run("reuses the sandbox when the machine server restarts the child", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, liveMachineServer(t))
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		marker := filepath.Join(h.layout().Session(testSession).Home(), "marker")
		require.NoError(t, os.WriteFile(marker, nil, 0o600))
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		assert.FileExists(t, marker)
	})
	t.Run("serves the proxy while Juggler runs", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/server/sessions" {
				_, _ = io.WriteString(w, "[]")
				return
			}
			_, _ = io.WriteString(w, r.URL.Path+" "+r.URL.RawQuery+" "+r.Header.Get(supervisorTokenHeader))
		}))
		t.Cleanup(upstream.Close)
		h := newHarness(t)
		h.childReady(t, strings.TrimPrefix(upstream.URL, "http://"))
		var body string
		h.runner.onRun = func(c Command) {
			if !slices.Contains(c.Argv, "--session-child") {
				return
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
				"http://"+c.Env[envSupervisorAddr]+"/api/server/lan?session=other", http.NoBody)
			require.NoError(t, err)
			req.Header.Set(supervisorTokenHeader, c.Env[envSupervisorToken])
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			body = string(data)
		}
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		assert.Equal(t, "/api/server/lan session="+testSession+" real-token", body)
	})
	t.Run("starts no proxy without a machine server address", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, "")
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		run := h.runner.runs[1]
		assert.Len(t, openPorts(run.Argv), 1, "only the child port")
		for _, key := range []string{envSupervisorAddr, envSupervisorToken, envSupervisorSession} {
			assert.NotContains(t, run.Env, key)
		}
	})
	t.Run("grants read access to a Juggler override", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, "")
		dev := filepath.Join(h.dir, "dev-juggler")
		require.NoError(t, os.WriteFile(dev, nil, 0o700))
		h.m.opt.Juggler = dev
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		run := h.runner.runs[1]
		assert.Equal(t, dev, argAfter(run.Argv, "--read-file"))
		assert.Equal(t, dev, argAfter(run.Argv, "--"))
	})
	t.Run("gives no GitHub credentials without a GitHub origin", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, "")
		require.NoError(t, os.RemoveAll(filepath.Join(h.project, ".git")))
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		assert.NotContains(t, readFile(t, h.layout().Session(testSession).Profile()), "github_api")
	})
	t.Run("reports a failing Juggler", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, "")
		h.runner.runErr = failWhen("--session-child", os.ErrInvalid)
		err := h.m.SessionChild(t.Context(), childArgs(h.project))
		require.ErrorIs(t, err, os.ErrInvalid)
		assert.Contains(t, err.Error(), "sandbox: session child:")
	})
	refusals := []struct {
		name    string
		mutate  func(h *harness) []string
		wantErr error
	}{
		{name: "no session id", wantErr: ErrNoSession, mutate: func(h *harness) []string {
			delete(h.env, envSupervisorSession)
			return childArgs(h.project)
		}},
		{name: "a session id that is a path", wantErr: ErrNoSession, mutate: func(h *harness) []string {
			h.env[envSupervisorSession] = "../" + testSession
			return childArgs(h.project)
		}},
		{name: "no --project", wantErr: ErrInvalidProject, mutate: func(*harness) []string {
			return []string{"--session-child", "--port", "0"}
		}},
		{name: "a project inside the state directory", wantErr: ErrInvalidProject, mutate: func(h *harness) []string {
			return childArgs(h.state)
		}},
		{name: "no tools.json", wantErr: ErrNoTools, mutate: func(h *harness) []string {
			_ = os.Remove(h.layout().ToolsFile())
			return childArgs(h.project)
		}},
		{name: "an unreviewed nono", wantErr: ErrNonoVersion, mutate: func(h *harness) []string {
			h.runner.version = "nono 0.82.0\n"
			return childArgs(h.project)
		}},
	}
	for _, tt := range refusals {
		t.Run("refuses "+tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.childReady(t, "")
			err := h.m.SessionChild(t.Context(), tt.mutate(h))
			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, h.runner.runs)
			assert.NoDirExists(t, h.layout().Session(testSession).Root())
		})
	}
}

func TestSessionChildTokenRotation(t *testing.T) {
	t.Run("ends the session when another sandbox rotates the tokens", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, "")
		h.runner.onRun = func(c Command) {
			if !slices.Contains(c.Argv, "--session-child") {
				return
			}
			writeToken(t, h.layout().ClaudeCredentials(), "rotated")
			select {
			case <-h.ended:
			case <-time.After(5 * time.Second):
				t.Error("the session did not end")
			}
		}
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		assert.Contains(t, h.errOut.String(), "the shared tokens changed")
	})
	t.Run("keeps the session when its own nono rotates the tokens", func(t *testing.T) {
		h := newHarness(t)
		h.childReady(t, "")
		h.runner.onRun = func(c Command) {
			if !slices.Contains(c.Argv, "--session-child") {
				return
			}
			appendFile(t, argAfter(c.Argv, "--log-file"), string(captureMark)+" provider=claude_code\n")
			writeToken(t, h.layout().CodexAuth(), "rotated")
			select {
			case <-h.ended:
				t.Error("the session ended")
			case <-time.After(50 * time.Millisecond):
			}
		}
		require.NoError(t, h.m.SessionChild(t.Context(), childArgs(h.project)))
		assert.Empty(t, h.errOut.String())
	})
}

// writeToken writes text to the identity file at path.
func writeToken(t *testing.T, path, text string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}

func TestFlagValue(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      string
		wantFound bool
	}{
		{name: "separate value", args: []string{"--project", "/p"}, want: "/p", wantFound: true},
		{name: "joined value", args: []string{"--project=/p"}, want: "/p", wantFound: true},
		{name: "the last one wins", args: []string{"--project", "/a", "--project=/b"}, want: "/b", wantFound: true},
		{name: "no value", args: []string{"--project"}},
		{name: "absent", args: []string{"--port", "0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := flagValue(tt.args, "--project")
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

func TestSetFlagValue(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "separate value", args: []string{"--log-file", "/a", "--dev"}, want: []string{"--log-file", "/new", "--dev"}},
		{name: "joined value", args: []string{"--log-file=/a"}, want: []string{"--log-file=/new"}},
		{name: "absent", args: []string{"--dev"}, want: []string{"--dev", "--log-file", "/new"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := slices.Clone(tt.args)
			assert.Equal(t, tt.want, setFlagValue(tt.args, "--log-file", "/new"))
			assert.Equal(t, original, tt.args, "the input is unchanged")
		})
	}
}
