package cmd

import (
	"slices"

	"github.com/spf13/cobra"
)

// sessionChildFlag is the first argument when a Juggler machine server runs
// this program as a session child.
const sessionChildFlag = "--session-child"

// sessionChildUse is the name of the hidden command for a session child.
const sessionChildUse = "session-child"

// SessionChildArgs returns the command-line arguments for the command tree.
// When args, the arguments after the program name, start with
// --session-child, it puts the hidden session-child command in front and
// reports true. Otherwise it returns args unchanged and false.
func SessionChildArgs(args []string) ([]string, bool) {
	if len(args) == 0 || args[0] != sessionChildFlag {
		return args, false
	}
	return slices.Concat([]string{sessionChildUse}, args), true
}

// newSessionChild returns the hidden command that runs a session child. It
// passes every argument to the Manager without parsing.
func newSessionChild(m Manager) *cobra.Command {
	return &cobra.Command{
		Use:                sessionChildUse,
		Short:              "Run a Juggler session child inside its sandbox (called by Juggler)",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return m.SessionChild(cmd.Context(), args)
		},
	}
}
