package sandbox

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// StartRequest is the input of [Manager.Start].
type StartRequest struct {
	// Port is the port of the Juggler UI. Juggler takes the first free port
	// at or after it.
	Port int
	// Project is a folder to open at start. Empty opens none; the user opens
	// folders from the page of the machine server.
	Project string
	// AllowHosts are extra Host names that the machine server accepts, such
	// as the name that tailscale serve forwards under. Juggler checks each name.
	AllowHosts []string
	// Extends names nono profiles that every session inherits after
	// "default". Each is a built-in, package, or user profile name. They can
	// widen the sandbox, so name only profiles that you trust.
	Extends []string
}

// Start replaces this process with a Juggler machine server on the host,
// outside nono. The machine server runs every session through this program
// (JUGGLER_SERVER_BIN), and [Manager.SessionChild] runs each session in its
// own nono sandbox. Start resolves the tools on PATH and writes them to
// machine/tools.json for the session children. The machine server keeps its
// state in machine/, separate from any other Juggler on the host.
//
// It returns [ErrInvalidPort] for a bad port, [ErrInvalidProfile] for an
// extra profile that is not a plain name, [ErrInvalidProject] for a
// project that a sandbox cannot grant, [ErrMissingTool] for a missing tool or
// an override that is not an executable file, and the errors that [Manager]
// lists. It returns an error wrapped with "sandbox:" when this program has no
// path, when ctx ends before the start, or when the exec fails.
func (m Manager) Start(ctx context.Context, req StartRequest) error {
	if err := req.validate(); err != nil {
		return err
	}
	l, err := m.layout()
	if err != nil {
		return err
	}
	project, err := m.startProject(req.Project, l)
	if err != nil {
		return err
	}
	juggler, err := m.prepareMachine(ctx, l)
	if err != nil {
		return err
	}
	env, err := m.machineEnvironment(l, juggler, req.Extends)
	if err != nil {
		return err
	}
	if err = m.say("Use the browser URL printed by Juggler. Ctrl-C stops the server and every session."); err != nil {
		return err
	}
	// A Ctrl-C during the steps above must not start Juggler.
	if err = ctx.Err(); err != nil {
		return fmt.Errorf("sandbox: start: %w", err)
	}
	run := Command{Argv: machineArgs(juggler, req, project), Env: env, Dir: m.opt.Home}
	if project != "" {
		run.Dir = project
	}
	if err = m.opt.Runner.Exec(run); err != nil {
		return fmt.Errorf("sandbox: start: %w", err)
	}
	return nil
}

// validate returns [ErrInvalidPort] for a bad port and [ErrInvalidProfile]
// for an extra profile that is not a plain name.
func (r StartRequest) validate() error {
	if err := ValidatePort(r.Port); err != nil {
		return err
	}
	return ValidateExtends(r.Extends)
}

// machineArgs returns the command line of the machine server.
func machineArgs(juggler string, req StartRequest, project string) []string {
	argv := []string{juggler, "--port", strconv.Itoa(req.Port)}
	// Only the machine server checks the Host header. Sessions save their
	// settings in identity/juggler, apart from the machine server, so the
	// allowed hosts come from the command line.
	for _, host := range req.AllowHosts {
		argv = append(argv, "--allow-host", host)
	}
	if project != "" {
		argv = append(argv, "--project", project)
	}
	return argv
}

// startProject returns the checked absolute path of project, or "" for none.
func (m Manager) startProject(project string, l Layout) (string, error) {
	if project == "" {
		return "", nil
	}
	abs, err := filepath.Abs(project)
	if err != nil {
		return "", fmt.Errorf("sandbox: resolve %s: %w", project, err)
	}
	return checkProject(abs, m.opt.Home, l.Roots()...)
}

// prepareMachine resolves the tools, checks nono, writes tools.json, and
// creates the shared identity. It returns the juggler executable to run.
func (m Manager) prepareMachine(ctx context.Context, l Layout) (string, error) {
	tools, err := m.resolveTools()
	if err != nil {
		return "", err
	}
	if err = m.checkNono(ctx, tools["nono"]); err != nil {
		return "", err
	}
	juggler, override, err := m.jugglerPath(tools)
	if err != nil {
		return "", err
	}
	if err = ensureDirs(l.State(), l.Machine()); err != nil {
		return "", err
	}
	if err = writeJSON(l.ToolsFile(), tools); err != nil {
		return "", err
	}
	if err = ensureIdentity(l); err != nil {
		return "", err
	}
	if override {
		return juggler, m.say("Using Juggler at " + juggler + ".")
	}
	return juggler, nil
}

// machineEnvironment returns the host environment of the machine server. It
// names this program as the session binary, machine/ as the configuration
// directory, juggler as the executable of every session, and the data and
// state directories for the session children.
func (m Manager) machineEnvironment(l Layout, juggler string, extends []string) (map[string]string, error) {
	self, err := m.opt.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	if err != nil {
		return nil, fmt.Errorf("sandbox: find this program: %w", err)
	}
	env := hostEnvironment()
	env["JUGGLER_SERVER_BIN"] = self
	env["JUGGLER_CONFIG_DIR"] = l.Machine()
	// The sessions must run the same Juggler build as the machine server.
	env[EnvJuggler] = juggler
	env[EnvExtends] = strings.Join(extends, " ")
	env[EnvDataDir] = l.Data()
	env[EnvStateDir] = l.State()
	return env, nil
}
