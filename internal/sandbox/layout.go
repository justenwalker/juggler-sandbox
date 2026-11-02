package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// codexConfig is the Codex configuration of the shared identity. Codex keeps
// its credentials in auth.json, which holds phantoms, and turns off the
// features whose endpoints the brokered route denies.
const codexConfig = `cli_auth_credentials_store = "file"

# The brokered route denies these ChatGPT endpoints. Disabling them avoids 403 noise.
[features]
apps = false
plugins = false
remote_plugin = false
tool_suggest = false

[analytics]
enabled = false
`

// brokeredHelper sends only the nono phantom. nono swaps in the real token.
const brokeredHelper = `"!f() { test $1 = get || exit 0; echo username=x-access-token; ` +
	`echo password=$GITHUB_GIT_TOKEN; }; f"`

// ensureDirs creates each directory with mode 0700. It returns [ErrSymlink]
// when one of them is a symbolic link.
func ensureDirs(dirs ...string) error {
	for _, dir := range dirs {
		if err := refuseSymlink(dir, "Refusing redirected directory"); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return fmt.Errorf("sandbox: create %s: %w", dir, err)
		}
	}
	return nil
}

// ensureIdentity creates the shared identity directories and the Codex
// configuration when they are missing.
func ensureIdentity(l Layout) error {
	if err := ensureDirs(l.Identity(), l.IdentityControl(), l.ClaudeDir(), l.CodexDir(), l.JugglerDir()); err != nil {
		return err
	}
	if _, err := os.Lstat(l.CodexConfig()); errors.Is(err, fs.ErrNotExist) {
		return writePrivate(l.CodexConfig(), codexConfig)
	}
	return nil
}

// ensureSession creates the directories, tool links, and Git configuration
// of the sandbox p. It replaces the tool links, so that p uses tools.
func ensureSession(p SessionPaths, tools Tools) error {
	err := ensureDirs(
		p.Root(), p.Control(), p.ControlLogs(), p.Runtime(), p.RuntimeBin(), p.RuntimeHooks(),
		p.Data(), p.Tmp(), p.Logs(), p.Home(), p.GHConfigDir(), p.CacheDir(),
	)
	if err != nil {
		return err
	}
	for _, name := range toolNames {
		link := p.ToolLink(name)
		if err = os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("sandbox: replace %s: %w", link, err)
		}
		if err = os.Symlink(tools[name], link); err != nil {
			return fmt.Errorf("sandbox: link %s: %w", name, err)
		}
	}
	return writeGitconfig(p)
}

// writeGitconfig writes runtime/gitconfig. Git takes GitHub credentials from
// nono and runs no hooks. GitHub SSH remotes use HTTPS.
func writeGitconfig(p SessionPaths) error {
	// SSH reaches neither the keys nor the network from a session. Over
	// HTTPS, the route of nono supplies the token.
	text := "[credential \"https://github.com\"]\n" +
		"\thelper =\n" +
		"\thelper = " + brokeredHelper + "\n" +
		"[url \"https://github.com/\"]\n" +
		"\tinsteadOf = git@github.com:\n" +
		"\tinsteadOf = ssh://git@github.com/\n" +
		"[core]\n" +
		"\thooksPath = \"" + p.RuntimeHooks() + "\"\n"
	return writePrivate(p.Gitconfig(), text)
}

// refuseSymlink returns [ErrSymlink] when path is a symbolic link.
// A path that does not exist is not a link.
func refuseSymlink(path, message string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sandbox: inspect %s: %w", path, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return withDetail(ErrSymlink, message+": "+path)
	}
	return nil
}
