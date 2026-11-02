package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// codexBundled is the Codex copy inside ChatGPT.app.
const codexBundled = "/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex"

// Provider names a service that a user can log in to.
type Provider string

// The providers that login accepts.
const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
	ProviderGitHub Provider = "github"
)

// Options configure [New]. Every field has a default.
type Options struct {
	// DataDir is the directory of the shared identity. Default:
	// $JUGGLER_SANDBOX_DATA_DIR, else $XDG_DATA_HOME/juggler-sandbox, else
	// <Home>/.local/share/juggler-sandbox.
	DataDir string
	// StateDir is the directory of the machine server state and the session
	// sandboxes. Default: $JUGGLER_SANDBOX_STATE_DIR, else
	// $XDG_STATE_HOME/juggler-sandbox, else <Home>/.local/state/juggler-sandbox.
	StateDir string
	// Home is the host home directory. Default: [os.UserHomeDir].
	Home string
	// Runner starts child processes. Default: [ExecRunner].
	Runner Runner
	// LookPath finds a tool on PATH. Default: [exec.LookPath].
	LookPath func(string) (string, error)
	// CodexBundled is the Codex path that is used when PATH has none.
	// Default: the copy in ChatGPT.app.
	CodexBundled string
	// Juggler is the path of a juggler executable that Start and
	// SessionChild run in place of the resolved one. The sandbox can read
	// this one file during a session. Default: empty.
	Juggler string
	// Getenv reads the environment of the machine server that spawned a
	// session child. Default: [os.Getenv].
	Getenv func(string) string
	// Executable returns the path of this program, which Start gives Juggler
	// as JUGGLER_SERVER_BIN. Default: [os.Executable].
	Executable func() (string, error)
	// ReadSecret reads a secret without echo and stops when ctx is done.
	// Default: a hidden prompt on /dev/tty, or on stdin when there is no terminal.
	ReadSecret func(ctx context.Context, prompt string) (string, error)
	// Stdout receives the messages of the program. Default: [os.Stdout].
	Stdout io.Writer
	// Stderr receives the warnings of a session child, whose stdout the
	// machine server reads. Default: [os.Stderr].
	Stderr io.Writer
	// Now returns the current time. Default: [time.Now].
	Now func() time.Time
	// RotationPoll is the interval at which a session child checks the
	// shared identity for tokens that another sandbox rotated. Default: 2s.
	RotationPoll time.Duration
	// EndSession ends a session child and every process of its session, so
	// that the machine server starts the session again. Default: kill the
	// process group that the session child leads.
	EndSession func() error
}

// Manager starts the machine server, logs in to providers, and runs session
// children. It holds no mutable state. The zero value is not usable. Call
// [New].
//
// Every error that leaves a method starts with "sandbox:" one time. Each
// method returns [ErrNoHome] when the data or state directory is unknown,
// [ErrSymlink] for a redirected directory under them, and [ErrNonoVersion]
// for a nono release other than the pinned one.
type Manager struct {
	opt Options
}

// The environment variables that name the data and state directories. Start
// sets them for the machine server, whose environment has no XDG variables.
const (
	EnvDataDir  = "JUGGLER_SANDBOX_DATA_DIR"
	EnvStateDir = "JUGGLER_SANDBOX_STATE_DIR"
)

// appName names the directories of the program under the XDG roots.
const appName = "juggler-sandbox"

// New returns a Manager. It fills each unset field of opt with its default.
func New(opt Options) Manager {
	if opt.Home == "" {
		// An unknown home leaves opt.Home empty. layout reports it.
		opt.Home, _ = os.UserHomeDir()
	}
	fillProcessDefaults(&opt)
	if opt.DataDir == "" {
		opt.DataDir = appDir(opt.Getenv, opt.Home, EnvDataDir, "XDG_DATA_HOME", ".local/share")
	}
	if opt.StateDir == "" {
		opt.StateDir = appDir(opt.Getenv, opt.Home, EnvStateDir, "XDG_STATE_HOME", ".local/state")
	}
	if opt.CodexBundled == "" {
		opt.CodexBundled = codexBundled
	}
	if opt.ReadSecret == nil {
		opt.ReadSecret = terminalSecret
	}
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	if opt.Stderr == nil {
		opt.Stderr = os.Stderr
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.RotationPoll <= 0 {
		opt.RotationPoll = defaultRotationPoll
	}
	return Manager{opt: opt}
}

// appDir returns the directory of the program under one XDG root: the
// absolute value of the variable own, else <xdg>/juggler-sandbox for an
// absolute xdg, else <home>/<fallback>/juggler-sandbox. The XDG specification
// ignores a relative value. It returns "" when every source is empty.
func appDir(getenv func(string) string, home, own, xdg, fallback string) string {
	if dir := getenv(own); filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	if dir := getenv(xdg); filepath.IsAbs(dir) {
		return filepath.Join(dir, appName)
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, fallback, appName)
}

// fillProcessDefaults fills the unset fields of opt that reach the operating
// system: Runner, LookPath, Getenv, Executable, and EndSession.
func fillProcessDefaults(opt *Options) {
	if opt.EndSession == nil {
		opt.EndSession = endProcessGroup
	}
	if opt.Runner == nil {
		opt.Runner = ExecRunner{}
	}
	if opt.LookPath == nil {
		opt.LookPath = exec.LookPath
	}
	if opt.Getenv == nil {
		opt.Getenv = os.Getenv
	}
	if opt.Executable == nil {
		opt.Executable = os.Executable
	}
}

// layout returns the Layout of the data and state directories. It returns
// [ErrNoHome] when either is unknown and [ErrSymlink] when one of them,
// machine/, identity/, identity/control/, or sessions/ is a symbolic link.
func (m Manager) layout() (Layout, error) {
	if m.opt.DataDir == "" || m.opt.StateDir == "" {
		return Layout{}, withDetail(ErrNoHome, "Set HOME, or "+EnvDataDir+" and "+EnvStateDir+".")
	}
	l := NewLayout(m.opt.DataDir, m.opt.StateDir)
	for _, path := range []string{l.Data(), l.State(), l.Machine(), l.Identity(), l.IdentityControl(), l.Sessions()} {
		if err := refuseSymlink(path, "Refusing redirected directory"); err != nil {
			return Layout{}, err
		}
	}
	return l, nil
}

// validate runs nono profile validate on profile.
func (m Manager) validate(ctx context.Context, nono, profile string) error {
	err := m.opt.Runner.Run(ctx, Command{
		Argv: []string{nono, "profile", "validate", profile},
		Env:  hostEnvironment(),
	})
	if err != nil {
		return fmt.Errorf("sandbox: validate profile: %w", err)
	}
	return nil
}

// say writes one line to the output of the Manager.
func (m Manager) say(line string) error {
	if _, err := fmt.Fprintln(m.opt.Stdout, line); err != nil {
		return fmt.Errorf("sandbox: write message: %w", err)
	}
	return nil
}
