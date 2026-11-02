package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// nonoVersion is the only nono release that this program targets.
const nonoVersion = "nono 0.81.0"

// toolNames lists the tools of a sandbox in the order that all code uses.
var toolNames = [...]string{"nono", "juggler", "node", "claude", "codex", "git", "gh"}

// Tools maps a tool name to its resolved absolute path.
type Tools map[string]string

// resolveTool returns the absolute path of the tool name with symbolic links
// resolved. For codex it falls back to the bundled copy in ChatGPT.app.
// It returns [ErrMissingTool] when no path is found.
func (m Manager) resolveTool(name string) (string, error) {
	path, err := m.opt.LookPath(name)
	if err != nil && name == "codex" && isExecutableFile(m.opt.CodexBundled) {
		path, err = m.opt.CodexBundled, nil
	}
	if err != nil {
		return "", withDetail(ErrMissingTool, "Install "+name+" and put it on PATH first.")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("sandbox: resolve %s: %w", name, err)
	}
	return resolved, nil
}

// resolveExplicitTool returns the absolute path of path with symbolic links
// resolved. It returns [ErrMissingTool] unless the result is a regular file
// with an execute bit.
func resolveExplicitTool(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("sandbox: resolve %s: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil || !isExecutableFile(resolved) {
		return "", withDetail(ErrMissingTool, "No executable file at "+abs+".")
	}
	return resolved, nil
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// resolveTools resolves every tool in toolNames.
func (m Manager) resolveTools() (Tools, error) {
	tools := make(Tools, len(toolNames))
	for _, name := range toolNames {
		path, err := m.resolveTool(name)
		if err != nil {
			return nil, err
		}
		tools[name] = path
	}
	return tools, nil
}

// loadTools reads the tool paths that Start wrote to tools.json. It returns
// [ErrNoTools] for a missing, redirected, or malformed file, or one that lacks
// a tool.
func loadTools(path string) (Tools, error) {
	detail := "Start the machine server with juggler-sandbox start, which writes " + path + "."
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, withDetail(ErrNoTools, detail)
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from Layout.
	if err != nil {
		return nil, fmt.Errorf("sandbox: read %s: %w", path, err)
	}
	var tools Tools
	if err = json.Unmarshal(data, &tools); err != nil {
		return nil, withDetail(ErrNoTools, detail)
	}
	for _, name := range toolNames {
		if tools[name] == "" {
			return nil, withDetail(ErrNoTools, detail)
		}
	}
	return tools, nil
}

// jugglerPath returns the juggler executable to run, and whether it is the
// override in [Options].Juggler. It returns [ErrMissingTool] when the
// override is not an executable file.
func (m Manager) jugglerPath(tools Tools) (string, bool, error) {
	if m.opt.Juggler == "" {
		return tools["juggler"], false, nil
	}
	path, err := resolveExplicitTool(m.opt.Juggler)
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

// checkNono runs nono --version. It returns [ErrNonoVersion] unless the
// version is the pinned release.
func (m Manager) checkNono(ctx context.Context, nono string) error {
	out, err := m.opt.Runner.Output(ctx, Command{Argv: []string{nono, "--version"}, Env: hostEnvironment()})
	if err != nil {
		return fmt.Errorf("sandbox: check nono: %w", err)
	}
	version := strings.TrimSpace(out)
	if version != nonoVersion {
		return withDetail(ErrNonoVersion,
			"This launcher targets "+nonoVersion+", found "+version+". Review compatibility first.")
	}
	return nil
}
