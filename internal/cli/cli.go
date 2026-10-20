// Package cli contains Garden's command entry point.
package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/garden"
	"github.com/mitschwimmer/gardener/internal/incusconn"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

// Run loads the compiled garden. Destructive commands read an explicit confirmation.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return run(args, stdin, stdout, stderr, garden.Definitions(), func() (reconcile.Backend, error) {
		server, err := incusconn.Connect()
		if err != nil {
			return nil, err
		}
		return incusconn.Backend{Server: server}, nil
	})
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, g definition.Garden, connect func() (reconcile.Backend, error)) error {
	flags := flag.NewFlagSet("garden", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: garden [-h] [plan|apply|adopt|delete]")
		fmt.Fprintln(stderr, "Garden manages a personal digital garden on Incus.")
		fmt.Fprintln(stderr, "plan validates and previews the entire garden without mutation.")
		fmt.Fprintln(stderr, "apply rediscovers and reconciles owned projects, networks, volumes, and VMs.")
		fmt.Fprintln(stderr, "adopt project NAME | adopt vm PROJECT NAME | adopt volume POOL NAME | adopt network NAME: validate and mark a declared resource as owned.")
		fmt.Fprintln(stderr, "delete project NAME | delete vm PROJECT NAME: confirm deletion of an owned orphan.")
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
	var target reconcile.Target
	if command == "adopt" || command == "delete" {
		switch {
		case command == "adopt" && flags.NArg() == 3 && flags.Arg(1) == "network":
			target.Network, target.Name = true, flags.Arg(2)
		case command == "adopt" && flags.NArg() == 4 && flags.Arg(1) == "volume":
			target.Pool, target.Name = flags.Arg(2), flags.Arg(3)
		case flags.NArg() == 3 && flags.Arg(1) == "project":
			target.Name = flags.Arg(2)
		case flags.NArg() == 4 && flags.Arg(1) == "vm":
			target.Project, target.Name = flags.Arg(2), flags.Arg(3)
		default:
			if command == "adopt" {
				return fmt.Errorf("usage: garden adopt project NAME | garden adopt vm PROJECT NAME | garden adopt volume POOL NAME | garden adopt network NAME")
			}
			return fmt.Errorf("usage: garden %s project NAME | garden %s vm PROJECT NAME", command, command)
		}
		if strings.HasPrefix(target.Pool, "-") || flags.Arg(1) == "volume" && target.Pool == "" || target.Name == "" || strings.HasPrefix(target.Name, "-") || flags.Arg(1) == "vm" && (target.Project == "" || strings.HasPrefix(target.Project, "-")) {
			return fmt.Errorf("explicit resource names are required")
		}
	} else if flags.NArg() != 1 || (command != "plan" && command != "apply") {
		return fmt.Errorf("unknown command %q; run garden -h for help", flags.Arg(0))
	}
	if err := g.Validate(); err != nil {
		return err
	}
	backend, err := connect()
	if err != nil {
		return err
	}
	if command == "adopt" || command == "delete" {
		var plan reconcile.Plan
		if command == "adopt" {
			plan, err = reconcile.BuildAdoption(g, target, backend)
		} else {
			plan, err = reconcile.BuildDeletion(g, target, backend)
		}
		if err != nil {
			return err
		}
		if command == "delete" {
			consequence := "Deletes the empty project."
			if target.Project != "" {
				consequence = "Deletes the stopped VM, its root disk, and its snapshots. Attached custom volumes are preserved."
			}
			if _, err := fmt.Fprintf(stdout, "%s %s\n%s\nType %q to confirm: ", command, target, consequence, target.String()); err != nil {
				return err
			}
			scanner := bufio.NewScanner(stdin)
			scanner.Buffer(make([]byte, 128), 1024)
			confirmed := scanner.Scan() && strings.TrimSpace(scanner.Text()) == target.String()
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read deletion confirmation: %w", err)
			}
			if !confirmed {
				return fmt.Errorf("deletion not confirmed; no changes made")
			}
		}
		return plan.Apply(backend, stdout)
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
