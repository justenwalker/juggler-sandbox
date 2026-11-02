package sandbox

import "path/filepath"

// Layout holds the two root directories and returns every shared path under
// them. The data directory holds the shared identity, which lasts. The state
// directory holds the machine server state and the session sandboxes, which
// the program deletes. The zero value is not usable. Call [NewLayout].
type Layout struct {
	data, state string
}

// NewLayout returns the Layout with the data directory data and the state
// directory state.
func NewLayout(data, state string) Layout { return Layout{data: data, state: state} }

// Data returns the data directory.
func (l Layout) Data() string { return l.data }

// State returns the state directory.
func (l Layout) State() string { return l.state }

// Roots returns the data and state directories.
func (l Layout) Roots() []string { return []string{l.data, l.state} }

// Machine returns the configuration directory of the machine server.
func (l Layout) Machine() string { return filepath.Join(l.state, "machine") }

// ToolsFile returns the file that holds the tool paths that start resolved.
func (l Layout) ToolsFile() string { return filepath.Join(l.Machine(), "tools.json") }

// Identity returns the directory of the shared provider identity.
func (l Layout) Identity() string { return filepath.Join(l.data, "identity") }

// IdentityControl returns the identity directory that no sandbox can read.
func (l Layout) IdentityControl() string { return filepath.Join(l.Identity(), "control") }

// GitHubToken returns the file that holds the GitHub token.
func (l Layout) GitHubToken() string { return filepath.Join(l.IdentityControl(), "github-token") }

// GitHubGitBasic returns the file that holds the GitHub basic-auth secret.
func (l Layout) GitHubGitBasic() string {
	return filepath.Join(l.IdentityControl(), "github-git-basic")
}

// ClaudeDir returns the Claude configuration directory of every session.
func (l Layout) ClaudeDir() string { return filepath.Join(l.Identity(), "claude") }

// ClaudeCredentials returns the Claude credential file, which holds phantoms.
func (l Layout) ClaudeCredentials() string { return filepath.Join(l.ClaudeDir(), ".credentials.json") }

// CodexDir returns the Codex home directory of every session.
func (l Layout) CodexDir() string { return filepath.Join(l.Identity(), "codex") }

// CodexConfig returns the Codex configuration file.
func (l Layout) CodexConfig() string { return filepath.Join(l.CodexDir(), "config.toml") }

// CodexAuth returns the Codex credential file, which holds phantoms.
func (l Layout) CodexAuth() string { return filepath.Join(l.CodexDir(), "auth.json") }

// JugglerDir returns the Juggler configuration directory of every session,
// which holds the Juggler settings of the user.
func (l Layout) JugglerDir() string { return filepath.Join(l.Identity(), "juggler") }

// Login returns the sandbox that login commands run in.
func (l Layout) Login() SessionPaths { return SessionPaths{root: filepath.Join(l.Identity(), "login")} }

// Sessions returns the directory of the session sandboxes.
func (l Layout) Sessions() string { return filepath.Join(l.state, "sessions") }

// Session returns the sandbox of the session id. The caller validates id.
func (l Layout) Session(id string) SessionPaths {
	return SessionPaths{root: filepath.Join(l.Sessions(), id)}
}

// SessionPaths holds the root of one sandbox and returns every path inside it.
// Get it from [Layout.Session] or [Layout.Login].
type SessionPaths struct {
	root string
}

// Root returns the sandbox directory.
func (p SessionPaths) Root() string { return p.root }

// Control returns the directory that the sandbox cannot read.
func (p SessionPaths) Control() string { return filepath.Join(p.root, "control") }

// Profile returns the nono profile file.
func (p SessionPaths) Profile() string { return filepath.Join(p.Control(), "profile.json") }

// ControlLogs returns the directory for nono logs.
func (p SessionPaths) ControlLogs() string { return filepath.Join(p.Control(), "logs") }

// ControlLog returns the nono log file of one action. The stamp is the start
// time of the action.
func (p SessionPaths) ControlLog(action, stamp string) string {
	return filepath.Join(p.ControlLogs(), action+"-"+stamp+".log")
}

// Runtime returns the directory that is read-only inside the sandbox.
func (p SessionPaths) Runtime() string { return filepath.Join(p.root, "runtime") }

// RuntimeBin returns the directory of tool links.
func (p SessionPaths) RuntimeBin() string { return filepath.Join(p.Runtime(), "bin") }

// RuntimeHooks returns the empty Git hooks directory.
func (p SessionPaths) RuntimeHooks() string { return filepath.Join(p.Runtime(), "hooks") }

// ToolLink returns the link to the tool name in the runtime bin directory.
func (p SessionPaths) ToolLink(name string) string { return filepath.Join(p.RuntimeBin(), name) }

// Gitconfig returns the Git configuration file of the sandbox.
func (p SessionPaths) Gitconfig() string { return filepath.Join(p.Runtime(), "gitconfig") }

// Data returns the directory that is read-write inside the sandbox.
func (p SessionPaths) Data() string { return filepath.Join(p.root, "data") }

// Tmp returns the temporary directory of the sandbox.
func (p SessionPaths) Tmp() string { return filepath.Join(p.Data(), "tmp") }

// Logs returns the Juggler log directory.
func (p SessionPaths) Logs() string { return filepath.Join(p.Data(), "logs") }

// Home returns the home directory of the sandbox.
func (p SessionPaths) Home() string { return filepath.Join(p.Data(), "home") }

// ConfigDir returns the XDG configuration directory.
func (p SessionPaths) ConfigDir() string { return filepath.Join(p.Home(), ".config") }

// GHConfigDir returns the GitHub CLI configuration directory.
func (p SessionPaths) GHConfigDir() string { return filepath.Join(p.ConfigDir(), "gh") }

// CacheDir returns the XDG cache directory.
func (p SessionPaths) CacheDir() string { return filepath.Join(p.Home(), ".cache") }

// Keychains returns the Keychain directory inside the sandbox home.
func (p SessionPaths) Keychains() string { return filepath.Join(p.Home(), "Library", "Keychains") }
