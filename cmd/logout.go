package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/config"
	"github.com/dental-dash/my-child-at-school-cli/internal/output"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored MCAS credentials from the OS keychain",
	RunE:  runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	format, err := resolveFormat(cfg)
	if err != nil {
		return err
	}
	if err := cfg.ClearCredentials(); err != nil {
		return emitFailure(format, "logout", output.APIErrorType, err)
	}
	return output.Result(os.Stdout, format, "logout", map[string]string{"status": "signed_out"}, func(w io.Writer, data any) error {
		_, err := fmt.Fprintln(w, "Signed out.")
		return err
	})
}
