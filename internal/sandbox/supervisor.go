package sandbox

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// The environment variables by which a Juggler machine server tells a session
// child how to reach it.
const (
	envSupervisorAddr    = "JUGGLER_SUPERVISOR_ADDR"
	envSupervisorToken   = "JUGGLER_SUPERVISOR_TOKEN"
	envSupervisorSession = "JUGGLER_SUPERVISOR_SESSION"
)

// supervisorTokenHeader carries the control-API token of the machine server.
const supervisorTokenHeader = "X-Juggler-Token" //nolint:gosec // G101: a header name, not a credential.

// supervisorTimeout bounds one request that the proxy passes on.
const supervisorTimeout = 2 * time.Minute

// supervisorRoutes are the machine-server routes that the proxy passes on.
// It passes GET requests only, so the sandbox can read the LAN and tunnel
// state but cannot change it.
var supervisorRoutes = []string{
	"/api/server/status",
	"/api/server/lan",
	"/api/server/tunnel",
	"/api/server/launch",
}

// supervisor is the machine server that spawned a session child.
type supervisor struct {
	// Addr is the loopback host:port of the machine server.
	Addr string
	// Token is the control-API token of the machine server.
	Token string
	// Session is the session id of the child at the machine server.
	Session string
}

// supervisorProxy is an [http.Handler] that stands in for the machine server.
// It accepts only requests with its own token, and passes the read-only
// routes on to the real machine server with the real token.
type supervisorProxy struct {
	target supervisor
	token  string
	client *http.Client
}

// ServeHTTP checks the token, the method, and the path of r. It refuses
// every other request with 403.
func (p supervisorProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(supervisorTokenHeader)), []byte(p.token)) != 1 {
		http.Error(w, "Invalid token.", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet || !slices.Contains(supervisorRoutes, r.URL.Path) {
		http.Error(w, "The sandbox does not allow this request.", http.StatusForbidden)
		return
	}
	// The child names its session in the query. Only the real session is passed on.
	target := "http://" + p.target.Addr + r.URL.Path
	if p.target.Session != "" {
		target += "?session=" + url.QueryEscape(p.target.Session)
	}
	ctx, cancel := context.WithTimeout(r.Context(), supervisorTimeout)
	defer cancel()
	//nolint:gosec // G704: the host is the machine server that spawned this program; the path is from the allowlist.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set(supervisorTokenHeader, p.target.Token)
	resp, err := p.client.Do(req) //nolint:gosec // G704: see the request above.
	if err != nil {
		http.Error(w, "The machine server could not be reached.", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// runningProxy is a supervisor proxy that serves on a loopback port.
type runningProxy struct {
	server   *http.Server
	listener net.Listener
	// token is the token that the session child must send.
	token string
}

// startSupervisorProxy serves a proxy for target on a free loopback port
// with a new random token. The caller must call close.
func startSupervisorProxy(ctx context.Context, target supervisor) (*runningProxy, error) {
	token, err := newSupervisorToken()
	if err != nil {
		return nil, err
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("sandbox: supervisor proxy: %w", err)
	}
	proxy := supervisorProxy{target: target, token: token, client: &http.Client{}}
	server := &http.Server{Handler: proxy, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return &runningProxy{server: server, listener: listener, token: token}, nil
}

// addr returns the host:port of the proxy.
func (p *runningProxy) addr() string { return p.listener.Addr().String() }

// port returns the port of the proxy.
func (p *runningProxy) port() int {
	addr, ok := p.listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0
	}
	return addr.Port
}

// close stops the proxy.
func (p *runningProxy) close() error {
	if err := p.server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("sandbox: close supervisor proxy: %w", err)
	}
	return nil
}

// newSupervisorToken returns 32 random bytes as hex.
func newSupervisorToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("sandbox: supervisor token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
