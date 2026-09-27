// Command mcas is a CLI for the My Child At School (MCAS) parent portal.
package main

import (
	"os"

	"github.com/dental-dash/my-child-at-school-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
