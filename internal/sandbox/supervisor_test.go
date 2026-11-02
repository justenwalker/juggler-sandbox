package sandbox

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upstreamCall is one request that the fake machine server received.
type upstreamCall struct {
	method, path, query, token string
}

// fakeMachineServer records each request and answers with status and body.
func fakeMachineServer(t *testing.T, status int, body string) (*httptest.Server, *[]upstreamCall) {
	t.Helper()
	var calls []upstreamCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, upstreamCall{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get(supervisorTokenHeader)})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

// proxyFor returns a proxy with the token "child-token" for the machine server at url.
func proxyFor(url, session string) supervisorProxy {
	target := supervisor{Addr: strings.TrimPrefix(url, "http://"), Token: "real-token", Session: session}
	return supervisorProxy{target: target, token: "child-token", client: http.DefaultClient}
}

// serve sends one request with token through p and returns the response.
func serve(t *testing.T, p supervisorProxy, method, target, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, http.NoBody)
	if token != "" {
		req.Header.Set(supervisorTokenHeader, token)
	}
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	return rec
}

func TestSupervisorProxy(t *testing.T) {
	t.Run("passes each read-only route with the real token and session", func(t *testing.T) {
		for _, route := range supervisorRoutes {
			upstream, calls := fakeMachineServer(t, http.StatusOK, `{"ok":true}`)
			rec := serve(t, proxyFor(upstream.URL, "s1"), http.MethodGet, route+"?session=other", "child-token")
			assert.Equal(t, http.StatusOK, rec.Code, route)
			assert.Equal(t, `{"ok":true}`, rec.Body.String())
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Equal(t, []upstreamCall{{http.MethodGet, route, "session=s1", "real-token"}}, *calls)
		}
	})
	t.Run("sends no session when the machine server named none", func(t *testing.T) {
		upstream, calls := fakeMachineServer(t, http.StatusOK, "{}")
		serve(t, proxyFor(upstream.URL, ""), http.MethodGet, "/api/server/status?session=other", "child-token")
		require.Len(t, *calls, 1)
		assert.Empty(t, (*calls)[0].query)
	})
	t.Run("passes the status of the machine server", func(t *testing.T) {
		upstream, _ := fakeMachineServer(t, http.StatusForbidden, `{"error":"no"}`)
		rec := serve(t, proxyFor(upstream.URL, "s1"), http.MethodGet, "/api/server/lan", "child-token")
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.JSONEq(t, `{"error":"no"}`, rec.Body.String())
	})
	refusals := []struct {
		name, method, target, token string
	}{
		{name: "no token", method: http.MethodGet, target: "/api/server/status"},
		{name: "the real token", method: http.MethodGet, target: "/api/server/status", token: "real-token"},
		{name: "a wrong token", method: http.MethodGet, target: "/api/server/status", token: "child-token-x"},
		{name: "a LAN switch", method: http.MethodPut, target: "/api/server/lan", token: "child-token"},
		{name: "a tunnel switch", method: http.MethodPut, target: "/api/server/tunnel", token: "child-token"},
		{name: "a launch switch", method: http.MethodPut, target: "/api/server/launch", token: "child-token"},
		{name: "a new session", method: http.MethodPost, target: "/api/server/sessions", token: "child-token"},
		{name: "the session list", method: http.MethodGet, target: "/api/server/sessions", token: "child-token"},
		{name: "a new folder", method: http.MethodPost, target: "/api/server/folders", token: "child-token"},
		{name: "a shutdown", method: http.MethodPost, target: "/api/server/shutdown", token: "child-token"},
		{name: "a path below a route", method: http.MethodGet, target: "/api/server/lan/x", token: "child-token"},
		{name: "the sessions page", method: http.MethodGet, target: "/", token: "child-token"},
	}
	for _, tt := range refusals {
		t.Run("refuses "+tt.name, func(t *testing.T) {
			upstream, calls := fakeMachineServer(t, http.StatusOK, "{}")
			rec := serve(t, proxyFor(upstream.URL, "s1"), tt.method, tt.target, tt.token)
			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.Empty(t, *calls, "the machine server sees nothing")
		})
	}
	t.Run("reports an unreachable machine server", func(t *testing.T) {
		upstream, _ := fakeMachineServer(t, http.StatusOK, "{}")
		upstream.Close()
		rec := serve(t, proxyFor(upstream.URL, "s1"), http.MethodGet, "/api/server/status", "child-token")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestStartSupervisorProxy(t *testing.T) {
	upstream, calls := fakeMachineServer(t, http.StatusOK, "{}")
	target := supervisor{Addr: strings.TrimPrefix(upstream.URL, "http://"), Token: "real-token", Session: "s1"}
	proxy, err := startSupervisorProxy(t.Context(), target)
	require.NoError(t, err)
	assert.Len(t, proxy.token, 64)
	assert.True(t, strings.HasPrefix(proxy.addr(), "127.0.0.1:"))
	assert.Positive(t, proxy.port())

	// get returns the status of one request through the proxy.
	get := func() (int, error) {
		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet,
			"http://"+proxy.addr()+"/api/server/status", http.NoBody)
		require.NoError(t, reqErr)
		req.Header.Set(supervisorTokenHeader, proxy.token)
		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			return 0, doErr
		}
		return resp.StatusCode, resp.Body.Close()
	}
	status, err := get()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Len(t, *calls, 1)

	require.NoError(t, proxy.close())
	_, err = get()
	require.Error(t, err, "a closed proxy accepts nothing")
}
