package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStart(t *testing.T) {
	const notice = "Use the browser URL printed by Juggler. Ctrl-C stops the server and every session.\n"
	t.Run("replaces the process with a host machine server", func(t *testing.T) {
		h := newHarness(t)
		require.NoError(t, h.m.Start(t.Context(), StartRequest{Port: 3941}))
		require.Len(t, h.runner.execs, 1)
		run := h.runner.execs[0]
		l := h.layout()
		assert.Equal(t, []string{h.tools["juggler"], "--port", "3941"}, run.Argv)
		assert.Equal(t, h.home, run.Dir)
		assert.Equal(t, h.self, run.Env["JUGGLER_SERVER_BIN"])
		assert.Equal(t, l.Machine(), run.Env["JUGGLER_CONFIG_DIR"])
		assert.Equal(t, h.tools["juggler"], run.Env[EnvJuggler])
		assert.Equal(t, h.data, run.Env[EnvDataDir], "session children find the identity")
		assert.Equal(t, h.state, run.Env[EnvStateDir], "session children find the sessions")
		assert.Equal(t, hostPath, run.Env["PATH"], "the machine server runs on the host")
		assert.Equal(t, notice, h.out.String())
		assert.Empty(t, h.runner.runs, "nothing runs in a sandbox")
	})
	t.Run("writes the tools and the shared identity", func(t *testing.T) {
		h := newHarness(t)
		require.NoError(t, h.m.Start(t.Context(), StartRequest{Port: 3939}))
		tools, err := loadTools(h.layout().ToolsFile())
		require.NoError(t, err)
		assert.Equal(t, h.tools, tools)
		for _, dir := range []string{h.layout().Machine(), h.layout().ClaudeDir(), h.layout().CodexDir()} {
			info, statErr := os.Stat(dir)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
		}
		assert.FileExists(t, h.layout().CodexConfig())
	})
	t.Run("opens a project", func(t *testing.T) {
		h := newHarness(t)
		require.NoError(t, h.m.Start(t.Context(), StartRequest{Port: 3939, Project: h.project}))
		run := h.runner.execs[0]
		assert.Equal(t, []string{h.tools["juggler"], "--port", "3939", "--project", h.project}, run.Argv)
		assert.Equal(t, h.project, run.Dir)
	})
	t.Run("passes each allowed host to the machine server", func(t *testing.T) {
		h := newHarness(t)
		req := StartRequest{Port: 3939, Project: h.project, AllowHosts: []string{"mac.example.ts.net", "other.example"}}
		require.NoError(t, h.m.Start(t.Context(), req))
		assert.Equal(t, []string{
			h.tools["juggler"], "--port", "3939",
			"--allow-host", "mac.example.ts.net", "--allow-host", "other.example",
			"--project", h.project,
		}, h.runner.execs[0].Argv)
	})
	t.Run("hands the extra profiles to the sessions", func(t *testing.T) {
		h := newHarness(t)
		req := StartRequest{Port: 3939, Extends: []string{"go-dev", "mine"}}
		require.NoError(t, h.m.Start(t.Context(), req))
		assert.Equal(t, "go-dev mine", h.runner.execs[0].Env[EnvExtends])
	})
	t.Run("refuses a profile path before it starts anything", func(t *testing.T) {
		h := newHarness(t)
		err := h.m.Start(t.Context(), StartRequest{Port: 3939, Extends: []string{"/tmp/p.json"}})
		require.ErrorIs(t, err, ErrInvalidProfile)
		assert.Empty(t, h.runner.execs)
	})
	t.Run("runs the Juggler override in the server and the sessions", func(t *testing.T) {
		h := newHarness(t)
		dev := filepath.Join(h.dir, "dev-juggler")
		require.NoError(t, os.WriteFile(dev, nil, 0o700))
		h.m.opt.Juggler = dev
		require.NoError(t, h.m.Start(t.Context(), StartRequest{Port: 3939}))
		assert.Equal(t, dev, h.runner.execs[0].Argv[0])
		assert.Equal(t, dev, h.runner.execs[0].Env[EnvJuggler])
		assert.Equal(t, "Using Juggler at "+dev+".\n"+notice, h.out.String())
	})
	refusals := []struct {
		name    string
		req     func(h *harness) StartRequest
		mutate  func(h *harness)
		wantErr error
	}{
		{name: "a bad port", req: func(*harness) StartRequest { return StartRequest{Port: 80} }, wantErr: ErrInvalidPort},
		{name: "the home as project", req: func(h *harness) StartRequest {
			return StartRequest{Port: 3939, Project: h.home}
		}, wantErr: ErrInvalidProject},
		{name: "a missing tool", mutate: func(h *harness) { delete(h.lookup, "node") }, wantErr: ErrMissingTool},
		{name: "an unreviewed nono", mutate: func(h *harness) { h.runner.version = "nono 0.82.0\n" }, wantErr: ErrNonoVersion},
		{name: "a missing override", mutate: func(h *harness) { h.m.opt.Juggler = "/no/such/juggler" }, wantErr: ErrMissingTool},
		{name: "a program without a path", mutate: func(h *harness) {
			h.m.opt.Executable = func() (string, error) { return "", os.ErrNotExist }
		}, wantErr: os.ErrNotExist},
		{name: "a redirected machine directory", mutate: func(h *harness) {
			_ = os.MkdirAll(h.state, 0o700)
			_ = os.Symlink(h.dir, h.layout().Machine())
		}, wantErr: ErrSymlink},
	}
	for _, tt := range refusals {
		t.Run("refuses "+tt.name, func(t *testing.T) {
			h := newHarness(t)
			if tt.mutate != nil {
				tt.mutate(h)
			}
			req := StartRequest{Port: 3939}
			if tt.req != nil {
				req = tt.req(h)
			}
			require.ErrorIs(t, h.m.Start(t.Context(), req), tt.wantErr)
			assert.Empty(t, h.runner.execs)
		})
	}
	t.Run("reports a failing exec", func(t *testing.T) {
		h := newHarness(t)
		h.runner.execErr = os.ErrInvalid
		require.ErrorIs(t, h.m.Start(t.Context(), StartRequest{Port: 3939}), os.ErrInvalid)
	})
	t.Run("does not exec after the context ends", func(t *testing.T) {
		h := newHarness(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, h.m.Start(ctx, StartRequest{Port: 3939}), context.Canceled)
		assert.Empty(t, h.runner.execs)
	})
}
