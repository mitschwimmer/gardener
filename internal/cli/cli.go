// Package cli contains Garden's command entry point.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/garden"
	"github.com/mitschwimmer/gardener/internal/incusconn"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

// Run loads the compiled garden and connects only for plan and apply.
func Run(args []string, stdout, stderr io.Writer) error {
	return run(args, stdout, stderr, garden.Definitions(), func() (reconcile.Backend, error) {
		server, err := incusconn.Connect()
		if err != nil {
			return nil, err
		}
		return incusconn.Backend{Server: server}, nil
	})
}

func run(args []string, stdout, stderr io.Writer, g definition.Garden, connect func() (reconcile.Backend, error)) error {
	flags := flag.NewFlagSet("garden", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: garden [-h] [plan|apply]")
		fmt.Fprintln(stderr, "Garden manages a personal digital garden on Incus.")
		fmt.Fprintln(stderr, "plan validates and previews the entire garden without mutation.")
		fmt.Fprintln(stderr, "apply rediscovers and reconciles owned projects and VMs.")
		fmt.Fprintln(stderr, "Definitions are compiled from internal/garden; remote: garden.")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() == 0 {
		_, err := fmt.Fprintln(stdout, "Run garden plan to preview or garden apply to reconcile the compiled definitions.")
		return err
	}
	command := flags.Arg(0)
	if flags.NArg() != 1 || (command != "plan" && command != "apply") {
		return fmt.Errorf("unknown command %q; run garden -h for help", flags.Arg(0))
	}
	if err := g.Validate(); err != nil {
		return err
	}
	backend, err := connect()
	if err != nil {
		return err
	}
	plan, err := reconcile.Build(g, backend)
	if err != nil {
		return err
	}
	if command == "apply" {
		return plan.Apply(backend, stdout)
	}
	if err := plan.Write(stdout); err != nil {
		return err
	}
	if len(plan.Manual) != 0 {
		return fmt.Errorf("plan requires %d manual actions", len(plan.Manual))
	}
	return nil
}
