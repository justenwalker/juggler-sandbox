// Package sandbox runs each Juggler session in its own nono sandbox on macOS.
package sandbox

import (
	"regexp"
	"strings"
)

const (
	minPort = 1024
	maxPort = 65535
	// portDetail states minPort and maxPort in the message text.
	portDetail = "Choose a port between 1024 and 65535."
)

var (
	repoPattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	sessionPattern = regexp.MustCompile(`^[0-9a-f]{8,64}$`)
)

// profileNamePattern matches the name of a nono profile: letters, digits,
// dots, dashes, and underscores. A path is not a name.
var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateExtends returns [ErrInvalidProfile] unless every element of names
// is the plain name of a nono profile.
func ValidateExtends(names []string) error {
	for _, name := range names {
		if !profileNamePattern.MatchString(name) {
			return withDetail(ErrInvalidProfile, "Name a built-in, package, or user profile, not a path.")
		}
	}
	return nil
}

// ValidatePort returns [ErrInvalidPort] unless port is between 1024 and 65535.
func ValidatePort(port int) error {
	if port < minPort || port > maxPort {
		return withDetail(ErrInvalidPort, portDetail)
	}
	return nil
}

// validSessionID reports whether id is a session id of a machine server:
// lowercase hex, 8 to 64 characters.
func validSessionID(id string) bool {
	return sessionPattern.MatchString(id)
}

// validRepo reports whether repo is OWNER/REPO, with no part equal to "." or
// "..".
func validRepo(repo string) bool {
	if !repoPattern.MatchString(repo) {
		return false
	}
	for part := range strings.SplitSeq(repo, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}
