package sandbox

import (
	"context"
	"fmt"
)

// rustLog turns on nono capture and TLS-interception debug logs.
const rustLog = "nono_proxy::oauth_capture=debug,nono_proxy::tls_intercept=debug"

// sandboxRequest names a sandbox to prepare.
type sandboxRequest struct {
	paths SessionPaths
	tools Tools
	// project is the folder that the sandbox can write. Empty for a login.
	project string
	// repo is the OWNER/REPO for GitHub credentials. Empty for none.
	repo string
	// extends names the nono profiles that the profile inherits after "default".
	extends []string
	// action names the nono log file.
	action string
}

// session is a prepared sandbox and the nono command that runs in it.
type session struct {
	paths SessionPaths
	// base is the nono run command up to, but not including, --workdir.
	base []string
	// env is the host environment. PATH starts with runtime/bin.
	env map[string]string
	// log is the nono log file of the run.
	log string
	// tokens are the identity files that nono reads or resolves phantoms
	// from. A session child ends its session when another nono changes one.
	tokens []string
}

// prepareSandbox creates the sandbox of req, writes and validates its
// profile, and builds its nono command. Capture and resolution logs go to
// control/logs, which the sandbox cannot read.
func (m Manager) prepareSandbox(ctx context.Context, l Layout, req sandboxRequest) (session, error) {
	if err := ensureSession(req.paths, req.tools); err != nil {
		return session{}, err
	}
	profile := BuildProfile(ProfileOptions{
		Layout: l, Session: req.paths, Home: m.opt.Home, Tools: req.tools,
		Project: req.project, Repo: req.repo, Extends: req.extends,
	})
	if err := writeJSON(req.paths.Profile(), profile); err != nil {
		return session{}, err
	}
	nono := req.tools["nono"]
	if err := m.validate(ctx, nono, req.paths.Profile()); err != nil {
		return session{}, err
	}
	env := hostEnvironment()
	env["PATH"] = req.paths.RuntimeBin() + ":" + env["PATH"]
	env["RUST_LOG"] = rustLog
	logFile := req.paths.ControlLog(req.action, m.opt.Now().Format("20060102-150405"))
	base := []string{
		nono, "run", "--log-file", logFile, "--profile", req.paths.Profile(),
		"--allow-cwd", "--no-rollback", "--strict-broker-path",
	}
	return session{paths: req.paths, base: base, env: env, log: logFile}, nil
}

// command returns the nono command that runs argv in workdir.
func (s session) command(workdir string, argv ...string) Command {
	full := append([]string{}, s.base...)
	full = append(full, "--workdir", workdir, "--")
	full = append(full, argv...)
	return Command{Argv: full, Env: s.env, Dir: workdir}
}

// warn writes err to the error output of the Manager. A session child uses
// it for failures that must not stop the session.
func (m Manager) warn(err error) {
	_, _ = fmt.Fprintf(m.opt.Stderr, "juggler-sandbox: %v\n", err)
}
