package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// listTimeout bounds the request for the live sessions.
const listTimeout = 10 * time.Second

// cleanupSessions deletes the sandbox of every session that the machine
// server target no longer lists, except keep. It does nothing without a
// machine server. It writes each failure to the error output and continues,
// because a failed cleanup must not stop a session.
func (m Manager) cleanupSessions(ctx context.Context, l Layout, target supervisor, keep string) {
	if target.Addr == "" || target.Token == "" {
		return
	}
	live, err := listSessions(ctx, target)
	if err != nil {
		m.warn(err)
		return
	}
	live[keep] = true
	entries, err := os.ReadDir(l.Sessions())
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			m.warn(fmt.Errorf("sandbox: read %s: %w", l.Sessions(), err))
		}
		return
	}
	for _, entry := range entries {
		id := entry.Name()
		if !validSessionID(id) || live[id] {
			continue
		}
		if err = os.RemoveAll(filepath.Join(l.Sessions(), id)); err != nil {
			m.warn(fmt.Errorf("sandbox: remove session %s: %w", id, err))
		}
	}
}

// listSessions returns the ids of the sessions that the machine server
// target lists.
func listSessions(ctx context.Context, target supervisor) (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+target.Addr+"/api/server/sessions", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("sandbox: list sessions: %w", err)
	}
	req.Header.Set(supervisorTokenHeader, target.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sandbox: list sessions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sandbox: list sessions: status %d", resp.StatusCode)
	}
	var sessions []struct {
		ID string `json:"id"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("sandbox: list sessions: %w", err)
	}
	live := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		live[s.ID] = true
	}
	return live, nil
}
