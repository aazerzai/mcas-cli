package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

var dinnerCmd = &cobra.Command{
	Use:   "dinner",
	Short: "Show the dinner money credit balance",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCommand("dinner", "balance", func(client *mcas.Client) (*mcas.DinnerBalance, error) {
			return client.DinnerBalance()
		}, renderDinnerText)
	},
}

func init() {
	rootCmd.AddCommand(dinnerCmd)
}

func renderDinnerText(w io.Writer, data any) error {
	balance, _ := data.(*mcas.DinnerBalance)
	if balance == nil {
		_, err := fmt.Fprintln(w, "No dinner balance published (the school may not use this module).")
		return err
	}
	_, err := fmt.Fprintf(w, "%.2f %s\n", balance.Amount, balance.Currency)
	return err
}
