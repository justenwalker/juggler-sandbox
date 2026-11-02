package sandbox

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

// caBundle is the system certificate bundle.
const caBundle = "/etc/ssl/cert.pem"

// Profile is a nono profile. It marshals to the JSON that nono reads.
type Profile struct {
	Extends             []string                      `json:"extends"`
	Meta                ProfileMeta                   `json:"meta"`
	Groups              ProfileGroups                 `json:"groups"`
	Workdir             ProfileWorkdir                `json:"workdir"`
	Filesystem          ProfileFilesystem             `json:"filesystem"`
	Environment         ProfileEnvironment            `json:"environment"`
	Network             ProfileNetwork                `json:"network"`
	CredentialProviders map[string]CredentialProvider `json:"credential_providers,omitempty"`
	CredentialRoutes    []CredentialRoute             `json:"credential_routes,omitempty"`
}

// ProfileMeta names the profile.
type ProfileMeta struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ProfileGroups lists the nono groups that the profile includes.
type ProfileGroups struct {
	Include []string `json:"include"`
}

// ProfileWorkdir sets the access to the working directory.
type ProfileWorkdir struct {
	Access string `json:"access"`
}

// ProfileFilesystem lists the paths that the sandbox may read, write, or not touch.
type ProfileFilesystem struct {
	Read             []string `json:"read"`
	ReadFile         []string `json:"read_file"`
	Allow            []string `json:"allow"`
	Deny             []string `json:"deny"`
	BypassProtection []string `json:"bypass_protection"`
	// SuppressSavePrompt lists the paths whose denials nono does not offer
	// to save to a profile after a run. It grants nothing and hides no
	// diagnostics.
	SuppressSavePrompt []string `json:"suppress_save_prompt,omitempty"`
}

// ProfileEnvironment lists the environment variables of the sandbox.
type ProfileEnvironment struct {
	AllowVars []string          `json:"allow_vars"`
	SetVars   map[string]string `json:"set_vars"`
}

// ProfileNetwork lists the network destinations of the sandbox.
type ProfileNetwork struct {
	AllowDomain       []string                    `json:"allow_domain"`
	OpenPort          []int                       `json:"open_port"`
	CustomCredentials map[string]CustomCredential `json:"custom_credentials,omitempty"`
	Credentials       []string                    `json:"credentials,omitempty"`
	TLSIntercept      *TLSIntercept               `json:"tls_intercept,omitempty"`
}

// TLSIntercept configures TLS interception in brokered mode.
type TLSIntercept struct {
	CAEnvVars []string `json:"ca_env_vars"`
}

// Rule allows one HTTP method on one path.
type Rule struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// CustomCredential injects a credential into requests to one upstream.
type CustomCredential struct {
	Upstream         string `json:"upstream"`
	CredentialKey    string `json:"credential_key"`
	EnvVar           string `json:"env_var"`
	InjectHeader     string `json:"inject_header,omitempty"`
	CredentialFormat string `json:"credential_format,omitempty"`
	InjectMode       string `json:"inject_mode,omitempty"`
	EndpointRules    []Rule `json:"endpoint_rules"`
}

// CredentialProvider captures OAuth tokens for one provider.
type CredentialProvider struct {
	Type            string          `json:"type"`
	TokenEndpoints  []TokenEndpoint `json:"token_endpoints"`
	APIHosts        []string        `json:"api_hosts"`
	CredentialStore CredentialStore `json:"credential_store"`
}

// TokenEndpoint is an OAuth token endpoint that nono intercepts.
type TokenEndpoint struct {
	Host               string          `json:"host"`
	Path               string          `json:"path"`
	ResponseFields     []ResponseField `json:"response_fields"`
	RequestBody        string          `json:"request_body"`
	RequestNonceFields []string        `json:"request_nonce_fields"`
}

// ResponseField is a token field in a token response.
type ResponseField struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Format string `json:"format,omitempty"`
}

// CredentialStore is the file where a provider keeps its phantom tokens.
type CredentialStore struct {
	Type          string   `json:"type"`
	Path          string   `json:"path"`
	PhantomFields []string `json:"phantom_fields"`
}

// CredentialRoute limits a provider to a set of requests.
type CredentialRoute struct {
	Name           string         `json:"name"`
	Provider       string         `json:"provider"`
	EndpointPolicy EndpointPolicy `json:"endpoint_policy"`
	Upgrades       []Upgrade      `json:"upgrades,omitempty"`
}

// EndpointPolicy is a default decision and the requests that are allowed.
type EndpointPolicy struct {
	Default Decision `json:"default"`
	Allow   []Rule   `json:"allow"`
}

// Decision is the default decision of an [EndpointPolicy].
type Decision struct {
	Decision string `json:"decision"`
}

// Upgrade names a WebSocket upgrade that a route permits.
type Upgrade struct {
	Origin string `json:"origin"`
	Path   string `json:"path"`
}

// ProfileOptions are the inputs of [BuildProfile].
type ProfileOptions struct {
	// Layout holds the shared identity and the machine server state.
	Layout Layout
	// Session is the sandbox that the profile is for.
	Session SessionPaths
	// Home is the host home directory.
	Home string
	// Tools holds the resolved tool paths.
	Tools Tools
	// Project is the project folder, read-write in the sandbox. Empty for a
	// login, which has no project.
	Project string
	// Repo is the OWNER/REPO that GitHub credentials are scoped to. Empty
	// gives the sandbox no GitHub credentials.
	Repo string
	// Extends names nono profiles to inherit after "default". They can widen
	// the sandbox: the launcher's own rules merge last, and nono cannot
	// remove an inherited path.
	Extends []string
}

// BuildProfile returns the nono profile for opt. It does no I/O.
//
// The profile inherits "default", then each profile in opt.Extends, in order.
//
// The sandbox can write the project, its own data directory, and the shared
// Claude, Codex, and Juggler directories. The Claude and Codex directories
// hold phantom tokens. nono keeps the real tokens and swaps them in on the
// allowed routes.
func BuildProfile(opt ProfileOptions) Profile {
	paths, layout := opt.Session, opt.Layout
	allow := []string{paths.Data(), layout.ClaudeDir(), layout.CodexDir(), layout.JugglerDir()}
	if opt.Project != "" {
		allow = append([]string{opt.Project}, allow...)
	}
	profile := Profile{
		Extends: append([]string{"default"}, opt.Extends...),
		Meta:    ProfileMeta{Name: "juggler-session", Version: "1.0.0"},
		Groups:  ProfileGroups{Include: []string{"node_runtime", "deny_keychains_macos"}},
		Workdir: ProfileWorkdir{Access: "readwrite"},
		Filesystem: ProfileFilesystem{
			Read: append(readPaths(opt.Tools), paths.Runtime()),
			// macOS system libraries read the user's default text encoding.
			ReadFile: []string{caBundle, filepath.Join(opt.Home, ".CFUserTextEncoding")},
			Allow:    allow,
			Deny: []string{
				paths.Control(),
				layout.IdentityControl(),
				layout.Machine(),
				filepath.Join(opt.Home, ".ssh"),
				filepath.Join(opt.Home, ".claude"),
				filepath.Join(opt.Home, ".codex"),
				filepath.Join(opt.Home, ".juggler"),
				filepath.Join(opt.Home, ".config", "gh"),
				filepath.Join(opt.Home, ".kube"),
				filepath.Join(opt.Home, ".aws"),
				filepath.Join(opt.Home, ".docker"),
				filepath.Join(opt.Home, "Library", "Keychains"),
				paths.Keychains(),
				"/var/run/docker.sock",
			},
			BypassProtection: []string{layout.ClaudeDir(), layout.CodexDir(), paths.GHConfigDir()},
			// The profile is generated on every run, so a saved grant would be lost.
			SuppressSavePrompt: []string{"/"},
		},
		Environment: ProfileEnvironment{
			AllowVars: []string{
				"PATH", "USER", "LOGNAME", "LANG", "TERM",
				// The machine server, through SessionChild, hands a session child these.
				envIngressSecret, envSessionToken,
				envSupervisorAddr, envSupervisorToken, envSupervisorSession,
			},
			SetVars: map[string]string{
				"HOME":                paths.Home(),
				"SHELL":               "/bin/bash",
				"TMPDIR":              paths.Tmp(),
				"XDG_CONFIG_HOME":     paths.ConfigDir(),
				"XDG_CACHE_HOME":      paths.CacheDir(),
				"JUGGLER_ENGINE_HOST": "node",
				"JUGGLER_CONFIG_DIR":  layout.JugglerDir(),
				"JUGGLER_LOG_DIR":     paths.Logs(),
				"CLAUDE_CONFIG_DIR":   layout.ClaudeDir(),
				"CODEX_HOME":          layout.CodexDir(),
				"GIT_CONFIG_NOSYSTEM": "1",
				"GIT_CONFIG_GLOBAL":   paths.Gitconfig(),
				"GIT_TERMINAL_PROMPT": "0",
				"GH_CONFIG_DIR":       paths.GHConfigDir(),
			},
		},
		Network: ProfileNetwork{
			AllowDomain: []string{
				"api.anthropic.com", "claude.ai", "claude.com",
				"platform.claude.com", "console.anthropic.com",
				"auth.openai.com", "chatgpt.com",
				"github.com", "api.github.com",
			},
			// No fixed port. SessionChild grants the port of its supervisor proxy per run.
			OpenPort: []int{},
		},
	}
	applyBroker(&profile, layout, opt.Repo)
	return profile
}

// readPaths returns the sorted unique tool paths. The .app bundle of each
// tool inside a bundle follows, in toolNames order, without duplicates.
func readPaths(tools Tools) []string {
	read := slices.Compact(slices.Sorted(maps.Values(tools)))
	// The server may use resources beside its executable in the app bundle.
	for _, name := range toolNames {
		path := tools[name]
		if !strings.Contains(path, ".app/Contents/") {
			continue
		}
		bundle, _, _ := strings.Cut(path, ".app/Contents/")
		bundle += ".app"
		if !slices.Contains(read, bundle) {
			read = append(read, bundle)
		}
	}
	return read
}
