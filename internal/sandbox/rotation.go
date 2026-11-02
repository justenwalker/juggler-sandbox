package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"os"
	"syscall"
	"time"
)

// defaultRotationPoll is the interval at which a session child checks the
// shared identity for new tokens.
const defaultRotationPoll = 2 * time.Second

// captureMark is the nono log message for a token response whose tokens nono
// replaced with phantoms.
var captureMark = []byte("rewrote OAuth token response fields")

// tokenWatch detects token changes in the shared identity that another nono
// process caused. The zero value is not usable. Call [newTokenWatch].
type tokenWatch struct {
	files    []string
	log      string
	sums     map[string][sha256.Size]byte
	captures int
}

// newTokenWatch returns a watch over files, which records the current
// contents. log is the nono log of this sandbox.
func newTokenWatch(files []string, log string) *tokenWatch {
	w := &tokenWatch{files: files, log: log}
	w.sums = w.read()
	w.captures = countCaptures(log)
	return w
}

// foreign reports whether a file changed since the last call while this
// nono captured no tokens. Only the nono that captured new phantoms can
// resolve them: each nono process loads the phantom map when it starts.
func (w *tokenWatch) foreign() bool {
	sums := w.read()
	if maps.Equal(sums, w.sums) {
		return false
	}
	w.sums = sums
	captures := countCaptures(w.log)
	own := captures > w.captures
	w.captures = captures
	return !own
}

// read returns the SHA-256 of each file. A file that cannot be read has the
// zero sum.
func (w *tokenWatch) read() map[string][sha256.Size]byte {
	sums := make(map[string][sha256.Size]byte, len(w.files))
	for _, path := range w.files {
		data, err := os.ReadFile(path) //nolint:gosec // G304: paths come from Layout.
		if err != nil {
			sums[path] = [sha256.Size]byte{}
			continue
		}
		sums[path] = sha256.Sum256(data)
	}
	return sums
}

// countCaptures returns the number of token captures in the nono log at path.
func countCaptures(path string) int {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from Layout.
	if err != nil {
		return 0
	}
	return bytes.Count(data, captureMark)
}

// watchTokens checks w until ctx ends. When the tokens change through
// another nono process, it ends the session with [Options].EndSession and
// returns. The machine server then starts the session again, in a nono that
// loads the new phantoms.
func (m Manager) watchTokens(ctx context.Context, w *tokenWatch) {
	ticker := time.NewTicker(m.opt.RotationPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !w.foreign() {
			continue
		}
		_, _ = fmt.Fprintln(m.opt.Stderr, "juggler-sandbox: the shared tokens changed; restarting the session")
		if err := m.opt.EndSession(); err != nil {
			m.warn(err)
		}
		return
	}
}

// endProcessGroup kills the process group that this process leads, this
// process included. It returns [ErrNoProcessGroup] when this process does not
// lead its group, because the group then holds other programs.
func endProcessGroup() error {
	if syscall.Getpgrp() != os.Getpid() {
		return withDetail(ErrNoProcessGroup, "Restart the session to load the new tokens.")
	}
	if err := syscall.Kill(0, syscall.SIGKILL); err != nil {
		return fmt.Errorf("sandbox: end the session: %w", err)
	}
	return nil
}
