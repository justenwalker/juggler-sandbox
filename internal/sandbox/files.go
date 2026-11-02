package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
)

// fileMode is the mode of every file that the program writes.
const fileMode = 0o600

// dirMode is the mode of every directory that the program creates.
const dirMode = 0o700

// writeJSON writes v to path as two-space indented JSON with a trailing
// newline, with the rules of [writePrivate].
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("sandbox: encode %s: %w", path, err)
	}
	return writePrivate(path, string(data)+"\n")
}

// writePrivate writes value to path with mode 0600. It truncates an existing
// file, sets the mode again, and refuses a symbolic link.
func writePrivate(path, value string) (err error) {
	// The path comes from Layout or SessionPaths, under the validated base.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, fileMode) //nolint:gosec // G304: path comes from Layout or SessionPaths.
	if err != nil {
		return fmt.Errorf("sandbox: open %s: %w", path, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if _, err = f.WriteString(value); err != nil {
		return fmt.Errorf("sandbox: write %s: %w", path, err)
	}
	if err = f.Chmod(fileMode); err != nil {
		return fmt.Errorf("sandbox: chmod %s: %w", path, err)
	}
	return nil
}
