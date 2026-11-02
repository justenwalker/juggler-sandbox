package sandbox

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func brokeredProfile() Profile {
	return BuildProfile(testProfileOptions())
}

// routeFor returns the credential route of provider.
func routeFor(t *testing.T, p Profile, provider string) CredentialRoute {
	t.Helper()
	for _, r := range p.CredentialRoutes {
		if r.Provider == provider {
			return r
		}
	}
	require.FailNow(t, "no route for provider", provider)
	return CredentialRoute{}
}

// The golden files pin the exact values. These tests state the properties
// that the security argument relies on.
func TestApplyBroker(t *testing.T) {
	t.Run("claude route denies by default and names every path", func(t *testing.T) {
		p := brokeredProfile()
		route := routeFor(t, p, "claude_code")
		assert.Equal(t, Decision{Decision: "deny"}, route.EndpointPolicy.Default)
		assert.NotContains(t, marshalProfile(t, Profile{CredentialRoutes: []CredentialRoute{route}}), "/**")
		assert.Empty(t, route.Upgrades)
	})
	t.Run("claude keeps the Keychain denied", func(t *testing.T) {
		p := brokeredProfile()
		assert.Contains(t, p.Groups.Include, "deny_keychains_macos")
		providers := marshalProfile(t, Profile{CredentialProviders: p.CredentialProviders})
		assert.NotContains(t, strings.ToLower(providers), "keychain")
	})
	t.Run("leaves the CA bundle to nono", func(t *testing.T) {
		p := brokeredProfile()
		assert.NotContains(t, p.Environment.SetVars, "SSL_CERT_FILE")
		assert.NotContains(t, p.Environment.SetVars, "CODEX_CA_CERTIFICATE")
		require.NotNil(t, p.Network.TLSIntercept)
	})
	t.Run("codex route denies by default and has no wildcard", func(t *testing.T) {
		p := brokeredProfile()
		route := routeFor(t, p, "codex_openai")
		assert.Equal(t, Decision{Decision: "deny"}, route.EndpointPolicy.Default)
		for _, rule := range route.EndpointPolicy.Allow {
			assert.NotContains(t, rule.Path, "*")
		}
		assert.NotEmpty(t, route.Upgrades, "the WebSocket upgrade needs a rule")
	})
	t.Run("codex token endpoints capture every token field", func(t *testing.T) {
		p := brokeredProfile()
		for _, endpoint := range p.CredentialProviders["codex_openai"].TokenEndpoints {
			kinds := map[string]string{}
			for _, f := range endpoint.ResponseFields {
				kinds[f.Path] = f.Kind
			}
			assert.Equal(t, map[string]string{
				"access_token": "opaque", "refresh_token": "opaque", "id_token": "jwt",
			}, kinds, endpoint.Path)
		}
	})
	t.Run("github credentials stay outside the sandbox and in one repository", func(t *testing.T) {
		p := brokeredProfile()
		controlURL := "file://" + testProfileOptions().Layout.IdentityControl()
		for name, cred := range p.Network.CustomCredentials {
			assert.True(t, strings.HasPrefix(cred.CredentialKey, controlURL), name)
			for _, rule := range cred.EndpointRules {
				ok := strings.Contains(rule.Path, testRepo) || rule == Rule{Method: "POST", Path: "/graphql"}
				assert.True(t, ok, "%s: %v", name, rule)
			}
		}
	})
	t.Run("adds no github credentials without a repository", func(t *testing.T) {
		opt := testProfileOptions()
		opt.Repo = ""
		p := BuildProfile(opt)
		assert.Empty(t, p.Network.CustomCredentials)
		assert.Empty(t, p.Network.Credentials)
		assert.NotNil(t, p.CredentialProviders["claude_code"], "Claude and Codex stay brokered")
	})
	t.Run("keeps the phantoms in the shared identity", func(t *testing.T) {
		l := testProfileOptions().Layout
		p := brokeredProfile()
		assert.Equal(t, l.ClaudeCredentials(), p.CredentialProviders["claude_code"].CredentialStore.Path)
		assert.Equal(t, l.CodexAuth(), p.CredentialProviders["codex_openai"].CredentialStore.Path)
	})
}
