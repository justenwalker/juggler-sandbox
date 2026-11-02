package sandbox

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"golang.org/x/term"
)

const verbLogin = "login"

const tokenPrompt = "Dedicated fine-grained GitHub token: "

// ttyPath is the controlling terminal of the process.
const ttyPath = "/dev/tty"

// Login authenticates one provider for the shared identity, which every
// session uses. One login per provider serves all sessions.
//
// For claude and codex, Login runs the login command of the provider inside
// nono, in the login sandbox under identity/. nono captures the real tokens
// and writes phantoms to the identity. For github, Login prompts for a token
// and writes it to identity/control, where nono reads it.
//
// It returns [ErrUnknownProvider] for a provider that has no login,
// [ErrMissingTool] for a missing tool, [ErrEmptyToken] for an empty GitHub
// token, [ErrNoCredentialFile] when Claude writes no credential file, and the
// errors that [Manager] lists. When ctx ends at the token prompt, it returns
// an error that wraps the error of ctx and writes nothing.
func (m Manager) Login(ctx context.Context, p Provider) error {
	l, err := m.layout()
	if err != nil {
		return err
	}
	if p == ProviderGitHub {
		if err = ensureIdentity(l); err != nil {
			return err
		}
		return m.storeGitHubToken(ctx, l)
	}
	s, argv, err := m.loginSandbox(ctx, l, p)
	if err != nil {
		return err
	}
	if err = m.say("Open any login URL manually in your host browser."); err != nil {
		return err
	}
	// An empty Stdin makes the runner connect the terminal to the login command.
	if err = m.opt.Runner.Run(ctx, s.command(s.paths.Data(), argv...)); err != nil {
		return fmt.Errorf("sandbox: login %s: %w", p, err)
	}
	if p == ProviderClaude {
		return checkClaudeCredentials(l)
	}
	return nil
}

// loginSandbox resolves the tools, checks nono, creates the identity, and
// prepares the login sandbox. It returns the session and the login command
// of p.
func (m Manager) loginSandbox(ctx context.Context, l Layout, p Provider) (session, []string, error) {
	tools, err := m.resolveTools()
	if err != nil {
		return session{}, nil, err
	}
	argv, err := loginCommand(tools, p)
	if err != nil {
		return session{}, nil, err
	}
	if err = m.checkNono(ctx, tools["nono"]); err != nil {
		return session{}, nil, err
	}
	if err = ensureIdentity(l); err != nil {
		return session{}, nil, err
	}
	s, err := m.prepareSandbox(ctx, l, sandboxRequest{paths: l.Login(), tools: tools, action: verbLogin})
	if err != nil {
		return session{}, nil, err
	}
	// A login runs at a terminal, where nono ends with a review prompt for
	// system-service denials. Silent mode skips that prompt and the banner.
	s.base = slices.Insert(slices.Clip(s.base), 2, "--silent")
	return s, argv, nil
}

// checkClaudeCredentials returns [ErrNoCredentialFile] unless Claude wrote a
// regular credential file.
func checkClaudeCredentials(l Layout) error {
	// Inspect the pathname only. Do not read or print the credential contents.
	info, err := os.Lstat(l.ClaudeCredentials())
	if err != nil || !info.Mode().IsRegular() {
		return withDetail(ErrNoCredentialFile, "Claude did not create "+l.ClaudeCredentials()+". "+
			"Keychain remains denied. Inspect CLI compatibility before continuing.")
	}
	return nil
}

// loginCommand returns the login command of p, which is claude or codex.
func loginCommand(tools Tools, p Provider) ([]string, error) {
	switch p {
	case ProviderClaude:
		return []string{tools["claude"], "auth", verbLogin}, nil
	case ProviderCodex:
		return []string{tools["codex"], "-c", `cli_auth_credentials_store="file"`, verbLogin, "--device-auth"}, nil
	case ProviderGitHub:
		return nil, withDetail(ErrUnknownProvider, "GitHub takes a token, not a login command.")
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, string(p))
	}
}

// storeGitHubToken prompts for a token and writes it where nono reads it.
func (m Manager) storeGitHubToken(ctx context.Context, l Layout) error {
	token, err := m.promptToken(ctx)
	if err != nil {
		return err
	}
	// nono reads these files outside the sandbox. No sandbox can read identity/control.
	for _, secret := range []struct{ path, value string }{
		{l.GitHubToken(), token},
		{l.GitHubGitBasic(), "x-access-token:" + token},
	} {
		// A Ctrl-C after the prompt must leave no partial secret behind.
		if err = ctx.Err(); err != nil {
			return fmt.Errorf("sandbox: store token: %w", err)
		}
		if err = writePrivate(secret.path, secret.value); err != nil {
			return err
		}
	}
	return m.say("Stored the GitHub token outside the sandboxes. Running sessions restart to use it.")
}

// promptToken reads the GitHub token and trims it. It returns [ErrEmptyToken]
// for an empty token.
func (m Manager) promptToken(ctx context.Context) (string, error) {
	token, err := m.opt.ReadSecret(ctx, tokenPrompt)
	if err != nil {
		return "", fmt.Errorf("sandbox: read token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", withDetail(ErrEmptyToken, "A GitHub token is required.")
	}
	return token, nil
}

// terminalSecret reads a secret without echo. It
// prompts on /dev/tty and falls back to stdin and stderr when the process has
// no terminal.
func terminalSecret(ctx context.Context, prompt string) (string, error) {
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return readSecret(ctx, os.Stdin, os.Stderr, prompt)
	}
	defer func() { _ = tty.Close() }()
	return readSecret(ctx, tty, tty, prompt)
}

// readSecret writes prompt to out and reads one secret from in. When in is
// a terminal, it reads without echo. Otherwise it reads one line.
//
// When ctx ends first, readSecret restores the terminal state and returns the
// error of ctx. The read goroutine then ends with its file, or with the
// process.
func readSecret(ctx context.Context, in *os.File, out io.Writer, prompt string) (string, error) {
	if _, err := io.WriteString(out, prompt); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	fd := int(in.Fd())
	read := func() (string, error) {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read secret: %w", err)
		}
		return line, nil
	}
	if term.IsTerminal(fd) {
		if state, err := term.GetState(fd); err == nil {
			defer func() { _ = term.Restore(fd, state) }()
		}
		read = func() (string, error) {
			secret, err := term.ReadPassword(fd)
			_, _ = io.WriteString(out, "\n")
			if err != nil {
				return "", fmt.Errorf("read secret: %w", err)
			}
			return string(secret), nil
		}
	}
	type result struct {
		secret string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		secret, err := read()
		done <- result{secret, err}
	}()
	select {
	case r := <-done:
		return r.secret, r.err
	case <-ctx.Done():
		return "", fmt.Errorf("read secret: %w", ctx.Err())
	}
}
