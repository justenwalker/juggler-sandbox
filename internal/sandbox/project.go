package sandbox

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxGitConfig bounds the part of .git/config that githubRepo reads.
const maxGitConfig = 1 << 20

// checkProject returns project as a clean absolute path. It returns
// [ErrInvalidProject] unless project is an existing directory that is not a
// symbolic link, and is neither /, home, nor a path that contains one of
// roots or is inside it.
func checkProject(project, home string, roots ...string) (string, error) {
	if !filepath.IsAbs(project) {
		return "", withDetail(ErrInvalidProject, "The project must be an absolute path: "+project)
	}
	project = filepath.Clean(project)
	info, err := os.Lstat(project)
	if err != nil || !info.IsDir() {
		return "", withDetail(ErrInvalidProject, "The project must be a directory, not a link: "+project)
	}
	if project == "/" || project == filepath.Clean(home) {
		return "", withDetail(ErrInvalidProject, "Open a project folder, not "+project+".")
	}
	for _, root := range roots {
		if within(project, root) || within(root, project) {
			return "", withDetail(ErrInvalidProject, "The project must be outside "+root+".")
		}
	}
	return project, nil
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// githubRepo returns the OWNER/REPO of the origin remote of the Git
// repository at project, when origin is on github.com. It returns "" for
// every other case. It reads .git/config as text and runs no Git command.
func githubRepo(project string) string {
	gitDir := filepath.Join(project, ".git")
	if info, err := os.Lstat(gitDir); err != nil || !info.IsDir() {
		return ""
	}
	f, err := os.Open(filepath.Join(gitDir, "config")) //nolint:gosec // G304: the project folder that the user opened.
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	return originRepo(io.LimitReader(f, maxGitConfig))
}

// originRepo returns the OWNER/REPO of the url in the [remote "origin"]
// section of a Git configuration, or "".
func originRepo(r io.Reader) string {
	inOrigin := false
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if inOrigin && ok && strings.TrimSpace(key) == "url" {
			return repoOfURL(strings.TrimSpace(value))
		}
	}
	return ""
}

// repoOfURL returns OWNER/REPO for a github.com remote URL in HTTPS, SSH, or
// scp-like form, or "".
func repoOfURL(url string) string {
	for _, prefix := range []string{
		"https://github.com/", "ssh://git@github.com/", "git@github.com:",
	} {
		if rest, ok := strings.CutPrefix(url, prefix); ok {
			repo := strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
			if validRepo(repo) {
				return repo
			}
			return ""
		}
	}
	return ""
}
