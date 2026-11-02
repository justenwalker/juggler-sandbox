package cmd

// Error is a constant sentinel error.
type Error string

// Error returns the text of the sentinel.
func (e Error) Error() string { return string(e) }

const (
	// ErrPlatform reports that the operating system is not macOS.
	ErrPlatform = Error("cmd: unsupported platform")

	// ErrNoCommand reports a call without a subcommand.
	ErrNoCommand = Error("cmd: no command given")

	// ErrPortText reports a --port value that is not a base-10 number.
	ErrPortText = Error("cmd: port is not a base-10 number")
)
