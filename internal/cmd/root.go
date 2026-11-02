// Package cmd is the Cobra command tree of juggler-sandbox.
//
// Argument errors from Cobra itself, such as a wrong argument count or an
// unknown provider, are already worded for the user. This package returns
// them without a wrap. The wrapcheck setting in .golangci.yaml states this
// policy once for every site.
package cmd

import (
	"context"
	"fmt"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/justenwalker/juggler-sandbox/internal/sandbox"
)

// umaskPrivate hides every file that the program creates from other users.
const umaskPrivate = 0o077

// Manager is the sandbox behavior that the commands call.
type Manager interface {
	Start(ctx context.Context, req sandbox.StartRequest) error
	Login(ctx context.Context, p sandbox.Provider) error
	SessionChild(ctx context.Context, args []string) error
}

// NewRootCommand returns the juggler-sandbox command tree. The commands call m.
func NewRootCommand(m Manager) *cobra.Command {
	return newRoot(m, runtime.GOOS, syscall.Umask)
}

// newRoot returns the command tree for the operating system goos. The tree
// calls setUmask before every command except help.
func newRoot(m Manager, goos string, setUmask func(int) int) *cobra.Command {
	root := &cobra.Command{
		Use:           "juggler-sandbox",
		Short:         "Run Juggler with each session in its own nono sandbox",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if goos != "darwin" {
				return fmt.Errorf("%w: %s", ErrPlatform, "This profile targets macOS Seatbelt only.")
			}
			setUmask(umaskPrivate)
			return nil
		},
		// A bare call is a usage error. Help flags never reach RunE.
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("%w: run juggler-sandbox --help to list the commands", ErrNoCommand)
		},
	}
	root.AddCommand(
		newStart(m),
		newLogin(m),
		newSessionChild(m),
	)
	return root
}
