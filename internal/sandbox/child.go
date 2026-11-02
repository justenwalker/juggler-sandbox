package sandbox

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// EnvJuggler names the environment variable that holds the path of a juggler
// executable for [Options].Juggler. Start sets it for the machine server, so
// that every session runs the same build.
const EnvJuggler = "JUGGLER_SANDBOX_JUGGLER"

// EnvExtends names the environment variable that holds the nono profiles
// that every session inherits after "default", separated by spaces. Start
// sets it for the machine server from [StartRequest].Extends.
const EnvExtends = "JUGGLER_SANDBOX_EXTENDS"

// The environment variables by which a machine server gives a session child
// its ingress secret and its API token.
//
//nolint:gosec // G101: environment variable names, not credentials.
const (
	envIngressSecret = "JUGGLER_INGRESS_SECRET"
	envSessionToken  = "JUGGLER_SESSION_TOKEN"
)

// SessionChild runs one Juggler session child in a nono sandbox of its own.
// A Juggler machine server calls this program in place of juggler when
// JUGGLER_SERVER_BIN names it. args are the arguments after the program
// name. They must contain --project with the project folder. SessionChild
// passes the other arguments to Juggler.
//
// The sandbox is sessions/<id>, for the id in JUGGLER_SUPERVISOR_SESSION.
// SessionChild creates it on the first spawn and reuses it when the machine
// server restarts the child. It deletes the sandboxes of sessions that the
// machine server no longer lists. The sandbox can write the project folder
// and the shared identity, which holds phantom tokens.
//
// SessionChild replaces the value of --log-file with data/logs/server.log,
// which the sandbox can write. It replaces the value of --port with a free
// port and lets the sandbox connect to that one port, so that processes in
// the sandbox reach the child. When the environment names a machine server,
// SessionChild serves a supervisor proxy and gives the child the address and
// token of the proxy, not those of the machine server. Through the proxy,
// the child can read the LAN and tunnel state, read-only. The output of
// SessionChild is the pipe on which the machine server reads the address of
// the child, so only Juggler writes to it.
//
// It returns [ErrNoSession] without a valid session id, [ErrInvalidProject]
// for a project that a sandbox cannot grant, [ErrNoTools] when Start wrote no
// tools.json, [ErrMissingTool] for an override that is not an executable
// file, and the errors that [Manager] lists. It returns an error wrapped with
// "sandbox:" when the proxy cannot start or when Juggler fails. That error
// wraps the error of [Runner].Run.
func (m Manager) SessionChild(ctx context.Context, args []string) error {
	s, juggler, project, err := m.childSession(ctx, args)
	if err != nil {
		return err
	}
	stop, err := m.supervise(ctx, &s)
	if err != nil {
		return err
	}
	defer stop()
	port, err := freeLoopbackPort(ctx)
	if err != nil {
		return err
	}
	// The engine host inside the sandbox connects to the child. nono can open
	// a port chosen here, but not the random port of --port 0.
	s.base = append(slices.Clip(s.base), "--open-port", strconv.Itoa(port))
	args = setFlagValue(args, "--port", strconv.Itoa(port))
	args = setFlagValue(args, "--log-file", filepath.Join(s.paths.Logs(), "server.log"))
	argv := append([]string{juggler}, args...)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go m.watchTokens(runCtx, newTokenWatch(s.tokens, s.log))
	if err = m.opt.Runner.Run(runCtx, s.command(project, argv...)); err != nil {
		return fmt.Errorf("sandbox: session child: %w", err)
	}
	return nil
}

// childSession checks the session id and the project, cleans up ended
// sessions, and prepares the sandbox of the session. It returns the session,
// the juggler executable to run, and the checked project folder. The
// environment of the command holds the ingress secret and the API token of
// the child.
func (m Manager) childSession(ctx context.Context, args []string) (session, string, string, error) {
	id := m.opt.Getenv(envSupervisorSession)
	if !validSessionID(id) {
		return session{}, "", "", withDetail(ErrNoSession, "Only a Juggler machine server can start a session child.")
	}
	l, err := m.layout()
	if err != nil {
		return session{}, "", "", err
	}
	project, ok := flagValue(args, "--project")
	if !ok {
		return session{}, "", "", withDetail(ErrInvalidProject, "The arguments have no --project.")
	}
	if project, err = checkProject(project, m.opt.Home, l.Roots()...); err != nil {
		return session{}, "", "", err
	}
	tools, juggler, err := m.childTools(ctx, l)
	if err != nil {
		return session{}, "", "", err
	}
	m.cleanupSessions(ctx, l, m.supervisorTarget(), id)
	s, err := m.prepareSandbox(ctx, l, sandboxRequest{
		paths: l.Session(id), tools: tools, project: project, repo: githubRepo(project),
		extends: m.childExtends(), action: "session",
	})
	if err != nil {
		return session{}, "", "", err
	}
	if juggler != tools["juggler"] {
		// The profile grants only the resolved juggler.
		s.base = append(slices.Clip(s.base), "--read-file", juggler)
	}
	for _, key := range []string{envIngressSecret, envSessionToken} {
		if value := m.opt.Getenv(key); value != "" {
			s.env[key] = value
		}
	}
	s.tokens = []string{l.ClaudeCredentials(), l.CodexAuth(), l.GitHubToken()}
	return s, juggler, project, nil
}

// childExtends returns the profile names that Start put in EnvExtends.
func (m Manager) childExtends() []string {
	return strings.Fields(m.opt.Getenv(EnvExtends))
}

// childTools reads tools.json, checks nono, and returns the tools and the
// juggler executable to run.
func (m Manager) childTools(ctx context.Context, l Layout) (Tools, string, error) {
	tools, err := loadTools(l.ToolsFile())
	if err != nil {
		return nil, "", err
	}
	if err = m.checkNono(ctx, tools["nono"]); err != nil {
		return nil, "", err
	}
	juggler, _, err := m.jugglerPath(tools)
	if err != nil {
		return nil, "", err
	}
	return tools, juggler, nil
}

// supervisorTarget returns the machine server that the environment names.
func (m Manager) supervisorTarget() supervisor {
	return supervisor{
		Addr:    m.opt.Getenv(envSupervisorAddr),
		Token:   m.opt.Getenv(envSupervisorToken),
		Session: m.opt.Getenv(envSupervisorSession),
	}
}

// supervise starts a supervisor proxy when the environment names a machine
// server. It points the child at the proxy and lets the sandbox connect to
// the proxy port. The returned function stops the proxy.
func (m Manager) supervise(ctx context.Context, s *session) (func(), error) {
	target := m.supervisorTarget()
	if target.Addr == "" || target.Token == "" {
		return func() {}, nil
	}
	proxy, err := startSupervisorProxy(ctx, target)
	if err != nil {
		return nil, err
	}
	s.base = append(slices.Clip(s.base), "--open-port", strconv.Itoa(proxy.port()))
	s.env[envSupervisorAddr] = proxy.addr()
	s.env[envSupervisorToken] = proxy.token
	s.env[envSupervisorSession] = target.Session
	return func() { _ = proxy.close() }, nil
}

// flagValue returns the value of the last flag name in args, given as
// "name value" or "name=value".
func flagValue(args []string, name string) (string, bool) {
	value, found := "", false
	for i, arg := range args {
		if v, ok := strings.CutPrefix(arg, name+"="); ok {
			value, found = v, true
		} else if arg == name && i+1 < len(args) {
			value, found = args[i+1], true
		}
	}
	return value, found
}

// setFlagValue returns a copy of args in which every value of the flag name
// is value. When args have no such flag, it appends "name value".
func setFlagValue(args []string, name, value string) []string {
	out := slices.Clone(args)
	found := false
	for i, arg := range out {
		if strings.HasPrefix(arg, name+"=") {
			out[i], found = name+"="+value, true
		} else if arg == name && i+1 < len(out) {
			out[i+1], found = value, true
		}
	}
	if !found {
		out = append(out, name, value)
	}
	return out
}

// freeLoopbackPort returns a loopback port that nothing listens on now.
func freeLoopbackPort(ctx context.Context) (int, error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("sandbox: find a free port: %w", err)
	}
	addr, _ := listener.Addr().(*net.TCPAddr)
	if err = listener.Close(); err != nil {
		return 0, fmt.Errorf("sandbox: find a free port: %w", err)
	}
	return addr.Port, nil
}
