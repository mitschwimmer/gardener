// Package cli contains Garden's command entry point.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// Run currently exposes help only. Infrastructure commands await MVP agreement.
func Run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("garden", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: garden [-h]")
		fmt.Fprintln(stderr, "Garden manages a personal digital garden on Incus.")
		fmt.Fprintln(stderr, "Infrastructure commands are not implemented yet.")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unknown command %q; run garden -h for help", flags.Arg(0))
	}
	_, err := fmt.Fprintln(stdout, "Garden is being designed. Run garden -h for help.")
	return err
}
