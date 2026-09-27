package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/dental-dash/my-child-at-school-cli/internal/config"
	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
	"github.com/dental-dash/my-child-at-school-cli/internal/output"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in to MCAS and store your credentials in the OS keychain",
	RunE:  runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	format, err := resolveFormat(cfg)
	if err != nil {
		return err
	}

	email := flagEmail
	if email == "" {
		email = os.Getenv("MCAS_EMAIL")
	}
	if email == "" {
		email, err = promptLine(cmd.InOrStdin(), cmd.OutOrStdout(), "Email: ")
		if err != nil {
			return err
		}
	}
	password := flagPassword
	if password == "" {
		password = os.Getenv("MCAS_PASSWORD")
	}
	if password == "" {
		password, err = promptPassword(cmd.OutOrStdout(), "Password: ")
		if err != nil {
			return err
		}
	}

	client := mcas.New(mcas.Credentials{Email: email, Password: password})
	if err := client.Login(); err != nil {
		errType := output.APIErrorType
		var authErr *mcas.AuthError
		if errors.As(err, &authErr) {
			errType = output.AuthErrorType
		}
		return emitFailure(format, "login", errType, err)
	}
	if err := cfg.SaveCredentials(email, password); err != nil {
		return emitFailure(format, "login", output.APIErrorType, fmt.Errorf("signed in, but could not save credentials: %w", err))
	}

	return output.Result(os.Stdout, format, "login", client.Session(), renderLoginText)
}

func renderLoginText(w io.Writer, data any) error {
	session := data.(mcas.Session)
	_, err := fmt.Fprintf(w, "Signed in as %s (%s). Credentials saved to the OS keychain.\n", session.StudentName, session.SchoolName)
	return err
}

func promptLine(in io.Reader, out io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptPassword(out io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// Not an interactive terminal (e.g. piped input in scripts/CI) -
		// fall back to a plain line read rather than failing outright.
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	bytes, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes)), nil
}
