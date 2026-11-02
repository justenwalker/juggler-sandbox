package sandbox

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRepo    = "plotly/dash-enterprise"
	testSession = "06ef2310"
)

// errAny in a wantErr field means that the case needs some error but has no
// sentinel to name.
var errAny = errors.New("any error")

// requireErr checks err against want. It accepts any error for errAny.
func requireErr(t *testing.T, err, want error) {
	t.Helper()
	if errors.Is(want, errAny) {
		require.Error(t, err)
		return
	}
	require.ErrorIs(t, err, want)
}

// fakeRunner records each Command and returns canned results.
type fakeRunner struct {
	version string
	outErr  error
	// runErr returns the error of a Run call. A nil func or a nil result means success.
	runErr  func(Command) error
	execErr error
	onRun   func(Command)
	outputs []Command
	runs    []Command
	execs   []Command
}

var _ Runner = (*fakeRunner)(nil)

func (f *fakeRunner) Run(_ context.Context, c Command) error {
	f.runs = append(f.runs, c)
	if f.onRun != nil {
		f.onRun(c)
	}
	if f.runErr == nil {
		return nil
	}
	return f.runErr(c)
}

func (f *fakeRunner) Output(_ context.Context, c Command) (string, error) {
	f.outputs = append(f.outputs, c)
	return f.version, f.outErr
}

func (f *fakeRunner) Exec(c Command) error {
	f.execs = append(f.execs, c)
	return f.execErr
}

// failWhen returns a runErr that fails with err for each Command whose Argv
// holds arg.
func failWhen(arg string, err error) func(Command) error {
	return func(c Command) error {
		if slices.Contains(c.Argv, arg) {
			return err
		}
		return nil
	}
}

// harness is a Manager that uses a temporary directory and a fake Runner.
type harness struct {
	m       Manager
	runner  *fakeRunner
	out     *bytes.Buffer
	errOut  *bytes.Buffer
	dir     string
	data    string
	state   string
	home    string
	project string
	tools   Tools
	token   string
	lookup  map[string]string
	// env is the environment that Options.Getenv reads.
	env map[string]string
	// self is the path that Options.Executable returns.
	self string
	// ended receives a value for each call of Options.EndSession.
	ended chan struct{}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o700))
	tools := Tools{}
	for _, name := range toolNames {
		path := filepath.Join(binDir, name)
		require.NoError(t, os.WriteFile(path, nil, 0o700))
		tools[name] = path
	}
	h := &harness{
		runner:  &fakeRunner{version: "nono 0.81.0\n"},
		out:     &bytes.Buffer{},
		errOut:  &bytes.Buffer{},
		dir:     dir,
		data:    filepath.Join(dir, "home", ".local", "share", "juggler-sandbox"),
		state:   filepath.Join(dir, "home", ".local", "state", "juggler-sandbox"),
		home:    filepath.Join(dir, "home"),
		project: filepath.Join(dir, "src", "app"),
		tools:   tools,
		token:   "test-token",
		lookup:  maps.Clone(tools),
		env:     map[string]string{},
		self:    filepath.Join(binDir, "juggler-sandbox"),
		ended:   make(chan struct{}, 1),
	}
	require.NoError(t, os.WriteFile(h.self, nil, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(h.project, ".git"), 0o700))
	gitConfig := "[remote \"origin\"]\n\turl = https://github.com/" + testRepo + ".git\n"
	require.NoError(t, os.WriteFile(filepath.Join(h.project, ".git", "config"), []byte(gitConfig), 0o600))
	// DataDir and StateDir take their defaults under the home.
	h.m = New(Options{
		Home:   h.home,
		Runner: h.runner,
		LookPath: func(name string) (string, error) {
			path, ok := h.lookup[name]
			if !ok {
				return "", exec.ErrNotFound
			}
			return path, nil
		},
		CodexBundled: filepath.Join(dir, "no-such-codex"),
		Getenv:       func(key string) string { return h.env[key] },
		Executable:   func() (string, error) { return h.self, nil },
		ReadSecret:   func(context.Context, string) (string, error) { return h.token, nil },
		Stdout:       h.out,
		Stderr:       h.errOut,
		Now:          func() time.Time { return time.Date(2026, 10, 30, 12, 34, 56, 0, time.UTC) },
		RotationPoll: 5 * time.Millisecond,
		EndSession: func() error {
			select {
			case h.ended <- struct{}{}:
			default:
			}
			return nil
		},
	})
	return h
}

func (h *harness) layout() Layout { return NewLayout(h.data, h.state) }

// writeTools writes tools.json, as Start does.
func (h *harness) writeTools(t *testing.T) {
	t.Helper()
	require.NoError(t, ensureDirs(h.state, h.layout().Machine()))
	require.NoError(t, writeJSON(h.layout().ToolsFile(), h.tools))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		opt       Options
		env       map[string]string
		wantData  string
		wantState string
	}{
		{
			name: "defaults under the home", opt: Options{Home: "/h"},
			wantData: "/h/.local/share/juggler-sandbox", wantState: "/h/.local/state/juggler-sandbox",
		},
		{
			name: "follows the XDG variables", opt: Options{Home: "/h"},
			env:      map[string]string{"XDG_DATA_HOME": "/xd", "XDG_STATE_HOME": "/xs"},
			wantData: "/xd/juggler-sandbox", wantState: "/xs/juggler-sandbox",
		},
		{
			name: "ignores relative XDG variables", opt: Options{Home: "/h"},
			env:      map[string]string{"XDG_DATA_HOME": "xd", "XDG_STATE_HOME": "xs"},
			wantData: "/h/.local/share/juggler-sandbox", wantState: "/h/.local/state/juggler-sandbox",
		},
		{
			name: "prefers its own variables", opt: Options{Home: "/h"},
			env: map[string]string{
				EnvDataDir: "/d", EnvStateDir: "/s/", "XDG_DATA_HOME": "/xd", "XDG_STATE_HOME": "/xs",
			},
			wantData: "/d", wantState: "/s",
		},
		{
			name: "keeps explicit directories", opt: Options{Home: "/h", DataDir: "/od", StateDir: "/os"},
			env:      map[string]string{EnvDataDir: "/d"},
			wantData: "/od", wantState: "/os",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opt.Getenv = func(key string) string { return tt.env[key] }
			m := New(tt.opt)
			assert.Equal(t, tt.wantData, m.opt.DataDir)
			assert.Equal(t, tt.wantState, m.opt.StateDir)
		})
	}
	t.Run("defaults the home to the user home", func(t *testing.T) {
		t.Setenv("HOME", "/users/test")
		assert.Equal(t, "/users/test", New(Options{}).opt.Home)
	})
}

func TestManagerLayout(t *testing.T) {
	t.Run("returns the layout of the data and state directories", func(t *testing.T) {
		h := newHarness(t)
		l, err := h.m.layout()
		require.NoError(t, err)
		assert.Equal(t, h.layout(), l)
	})
	t.Run("needs both directories", func(t *testing.T) {
		_, err := Manager{opt: Options{DataDir: "/d"}}.layout()
		require.ErrorIs(t, err, ErrNoHome)
	})
	for _, rel := range []string{"data", "state", "state/machine", "data/identity", "data/identity/control", "state/sessions"} {
		t.Run("refuses a redirected "+rel, func(t *testing.T) {
			h := newHarness(t)
			root, sub, _ := strings.Cut(rel, "/")
			path := map[string]string{"data": h.data, "state": h.state}[root]
			path = filepath.Join(path, sub)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.Symlink(h.dir, path))
			_, err := h.m.layout()
			require.ErrorIs(t, err, ErrSymlink)
		})
	}
}
