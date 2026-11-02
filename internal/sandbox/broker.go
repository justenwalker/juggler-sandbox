package sandbox

import "net/http"

const (
	codexResponses    = "/backend-api/codex/responses"
	fieldAccessToken  = "access_token"
	fieldRefreshToken = "refresh_token"
	kindOpaque        = "opaque"
)

// claudeRoutes are the Anthropic API calls that may carry the resolved
// Claude token. nono denies every other call.
var claudeRoutes = []Rule{
	{Method: http.MethodPost, Path: "/v1/messages"},
	{Method: http.MethodPost, Path: "/v1/messages/count_tokens"},
	{Method: http.MethodGet, Path: "/api/oauth/profile"},
	{Method: http.MethodGet, Path: "/api/oauth/claude_cli/roles"},
	{Method: http.MethodGet, Path: "/api/oauth/account/settings"},
	{Method: http.MethodGet, Path: "/api/oauth/usage"},
	{Method: http.MethodGet, Path: "/api/claude_code/policy_limits"},
	{Method: http.MethodGet, Path: "/api/hello"},
}

// codexRoutes are the Codex requests that the route permits.
// GET on responses is the WebSocket upgrade.
var codexRoutes = []Rule{
	{Method: http.MethodPost, Path: codexResponses},
	{Method: http.MethodGet, Path: codexResponses},
	{Method: http.MethodGet, Path: "/backend-api/codex/models"},
	{Method: http.MethodGet, Path: "/backend-api/codex/usage"},
	{Method: http.MethodGet, Path: "/backend-api/wham/usage"},
	// These three rules allow device login on auth.openai.com.
	// The policy applies to every request on the hosts of the route.
	{Method: http.MethodPost, Path: "/api/accounts/deviceauth/usercode"},
	{Method: http.MethodPost, Path: "/api/accounts/deviceauth/token"},
	{Method: http.MethodPost, Path: "/oauth/token"},
}

// codexTokenFields are the token fields in a Codex token response.
var codexTokenFields = []ResponseField{
	{Path: fieldAccessToken, Kind: kindOpaque},
	{Path: fieldRefreshToken, Kind: kindOpaque},
	{Path: "id_token", Kind: "jwt"},
}

// githubCredentials returns the nono credentials that use the GitHub token
// only for repo.
func githubCredentials(layout Layout, repo string) map[string]CustomCredential {
	api := []Rule{
		{Method: http.MethodGet, Path: "/repos/" + repo},
		{Method: http.MethodGet, Path: "/repos/" + repo + "/**"},
		{Method: http.MethodPost, Path: "/repos/" + repo + "/pulls"},
		{Method: http.MethodPatch, Path: "/repos/" + repo + "/pulls/*"},
		{Method: http.MethodPost, Path: "/repos/" + repo + "/issues/*/comments"},
		// gh pr uses GraphQL, which paths cannot scope. The token's own scope applies.
		{Method: http.MethodPost, Path: "/graphql"},
	}
	git := []Rule{
		{Method: http.MethodGet, Path: "/" + repo + ".git/info/refs"},
		{Method: http.MethodPost, Path: "/" + repo + ".git/git-upload-pack"},
		{Method: http.MethodPost, Path: "/" + repo + ".git/git-receive-pack"},
	}
	return map[string]CustomCredential{
		"github_api": {
			Upstream:         "https://api.github.com",
			CredentialKey:    "file://" + layout.GitHubToken(),
			EnvVar:           "GH_TOKEN",
			InjectHeader:     "Authorization",
			CredentialFormat: "token {}",
			EndpointRules:    api,
		},
		"github_git": {
			Upstream:      "https://github.com",
			CredentialKey: "file://" + layout.GitHubGitBasic(),
			EnvVar:        "GITHUB_GIT_TOKEN",
			InjectMode:    "basic_auth",
			EndpointRules: git,
		},
	}
}

// applyBroker adds nono brokering to p, so that the sandbox holds only
// phantom Claude, Codex, and GitHub tokens. The phantoms are in the shared
// identity of layout. GitHub credentials apply to repo only, and an empty
// repo adds none.
func applyBroker(p *Profile, layout Layout, repo string) {
	if repo != "" {
		p.Network.CustomCredentials = githubCredentials(layout, repo)
		p.Network.Credentials = []string{"github_api", "github_git"}
	}
	// nono points the standard CA variables at its interception bundle.
	p.Network.TLSIntercept = &TLSIntercept{CAEnvVars: []string{"CODEX_CA_CERTIFICATE"}}

	codexEndpoints := make([]TokenEndpoint, 0, 2)
	for _, path := range []string{"/api/accounts/deviceauth/token", "/oauth/token"} {
		// Device auth polls one endpoint and exchanges at the other.
		codexEndpoints = append(codexEndpoints, TokenEndpoint{
			Host:               "https://auth.openai.com",
			Path:               path,
			ResponseFields:     codexTokenFields,
			RequestBody:        "auto",
			RequestNonceFields: []string{fieldRefreshToken},
		})
	}
	p.CredentialProviders = map[string]CredentialProvider{
		"claude_code": {
			Type: "oauth_capture",
			TokenEndpoints: []TokenEndpoint{{
				Host: "https://platform.claude.com",
				Path: "/v1/oauth/token",
				ResponseFields: []ResponseField{
					{Path: fieldAccessToken, Kind: kindOpaque, Format: "sk-ant-oat01-{}"},
					{Path: fieldRefreshToken, Kind: kindOpaque},
				},
				RequestBody:        "json",
				RequestNonceFields: []string{fieldAccessToken, fieldRefreshToken},
			}},
			APIHosts: []string{"https://api.anthropic.com"},
			CredentialStore: CredentialStore{
				Type:          "file_json",
				Path:          layout.ClaudeCredentials(),
				PhantomFields: []string{"claudeAiOauth.accessToken", "claudeAiOauth.refreshToken"},
			},
		},
		"codex_openai": {
			Type:           "oauth_capture",
			TokenEndpoints: codexEndpoints,
			// The auth host needs a route for its other device-auth calls, or nono returns 502.
			APIHosts: []string{"https://chatgpt.com", "https://auth.openai.com"},
			CredentialStore: CredentialStore{
				Type: "file_json",
				Path: layout.CodexAuth(),
				PhantomFields: []string{
					"tokens.id_token", "tokens.access_token", "tokens.refresh_token",
				},
			},
		},
	}
	p.CredentialRoutes = []CredentialRoute{
		route("claude_oauth", "claude_code", claudeRoutes, nil),
		route("codex_chatgpt", "codex_openai", codexRoutes, []Upgrade{
			{Origin: "https://chatgpt.com", Path: codexResponses},
		}),
	}
}

// route returns a credential route that denies everything but allowed.
func route(name, provider string, allowed []Rule, upgrades []Upgrade) CredentialRoute {
	return CredentialRoute{
		Name:     name,
		Provider: provider,
		EndpointPolicy: EndpointPolicy{
			Default: Decision{Decision: "deny"},
			Allow:   allowed,
		},
		// nono rejects every WebSocket upgrade that no rule names.
		Upgrades: upgrades,
	}
}
