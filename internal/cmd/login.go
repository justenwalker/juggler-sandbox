package cmd

import (
	"github.com/spf13/cobra"

	"github.com/justenwalker/juggler-sandbox/internal/sandbox"
)

// newLogin returns the login command.
func newLogin(m Manager) *cobra.Command {
	return &cobra.Command{
		Use:       "login {claude|codex|github}",
		Short:     "Log in once for every session; nono keeps the real tokens",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{string(sandbox.ProviderClaude), string(sandbox.ProviderCodex), string(sandbox.ProviderGitHub)},
		RunE: func(cmd *cobra.Command, args []string) error {
			return m.Login(cmd.Context(), sandbox.Provider(args[0]))
		},
	}
}
