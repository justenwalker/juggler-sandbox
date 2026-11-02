package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/justenwalker/juggler-sandbox/internal/sandbox"
)

// defaultPort is the default port of the Juggler UI.
const defaultPort = 3939

// portValue is a pflag.Value that parses a base-10 port. The stock int flag
// also accepts 0x, 0o, and leading-zero octal text.
type portValue struct{ port *int }

// String returns the port as text.
func (v portValue) String() string {
	if v.port == nil {
		return ""
	}
	return strconv.Itoa(*v.port)
}

// Set parses s as a base-10 number. It returns [ErrPortText] otherwise.
func (v portValue) Set(s string) error {
	port, err := strconv.ParseInt(s, 10, 0)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrPortText, s)
	}
	*v.port = int(port)
	return nil
}

// Type returns the type name for the help text.
func (portValue) Type() string { return "int" }

// newStart returns the start command.
func newStart(m Manager) *cobra.Command {
	req := sandbox.StartRequest{Port: defaultPort}
	cmd := &cobra.Command{
		Use:   "start [PROJECT]",
		Short: "Run the Juggler machine server; each session runs in its own sandbox",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(*cobra.Command, []string) error {
			if err := sandbox.ValidatePort(req.Port); err != nil {
				return err
			}
			return sandbox.ValidateExtends(req.Extends)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				req.Project = args[0]
			}
			return m.Start(cmd.Context(), req)
		},
	}
	cmd.Flags().Var(portValue{&req.Port}, "port", "port of the Juggler UI, 1024 to 65535")
	cmd.Flags().StringArrayVar(&req.AllowHosts, "allow-host", nil,
		"also accept requests for this host name, such as the name of tailscale serve; repeatable")
	cmd.Flags().StringArrayVar(&req.Extends, "extends", nil,
		"nono profile that every session inherits after default, such as go-dev; repeatable; can widen the sandbox")
	return cmd
}
