package sandbox

import "fmt"

// Error is a constant sentinel error.
type Error string

// Error returns the text of the sentinel.
func (e Error) Error() string { return string(e) }

const (
	// ErrNoHome reports that the home directory of the user is unknown.
	ErrNoHome = Error("sandbox: home directory unknown")

	// ErrInvalidPort reports a port outside 1024 to 65535.
	ErrInvalidPort = Error("sandbox: invalid port")

	// ErrInvalidProfile reports a nono profile name that is not a plain name.
	ErrInvalidProfile = Error("sandbox: invalid profile name")

	// ErrUnknownProvider reports a login provider that has no command.
	ErrUnknownProvider = Error("sandbox: unknown provider")

	// ErrSymlink reports a sandbox path or a control file that is a symbolic link.
	ErrSymlink = Error("sandbox: refusing symbolic link")

	// ErrMissingTool reports a tool that is not on PATH, or a tool path that
	// is not an executable file.
	ErrMissingTool = Error("sandbox: tool not found")

	// ErrNoTools reports a missing or damaged tools.json, which start writes.
	ErrNoTools = Error("sandbox: no tool paths")

	// ErrNonoVersion reports a nono release other than the pinned version.
	ErrNonoVersion = Error("sandbox: unsupported nono version")

	// ErrEmptyToken reports an empty GitHub token.
	ErrEmptyToken = Error("sandbox: empty token")

	// ErrNoCredentialFile reports that Claude login wrote no credential file.
	ErrNoCredentialFile = Error("sandbox: no Claude credential file")

	// ErrNoSession reports a session child without a valid
	// JUGGLER_SUPERVISOR_SESSION.
	ErrNoSession = Error("sandbox: no session id")

	// ErrInvalidProject reports a project folder that a sandbox cannot grant.
	ErrInvalidProject = Error("sandbox: invalid project")

	// ErrNoProcessGroup reports a session child that does not lead its
	// process group, so it cannot end its session.
	ErrNoProcessGroup = Error("sandbox: not a process group leader")
)

// withDetail returns sentinel followed by a one-sentence detail for the user.
// The detail is a capitalized sentence that ends with a period.
func withDetail(sentinel Error, detail string) error {
	return fmt.Errorf("%w: %s", sentinel, detail)
}
