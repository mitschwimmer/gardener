package reconcile

import (
	"errors"
	"fmt"
	"strings"
)

// quoteCommandArg keeps diagnostic commands copyable without interpreting names
// as shell syntax. Commands are displayed only, never executed by Garden.
func quoteCommandArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (op Operation) recovery() string {
	switch op.Kind {
	case AdoptProject:
		return "garden adopt project " + quoteCommandArg(op.Name)
	case AdoptVM:
		return "garden adopt vm " + quoteCommandArg(op.Project) + " " + quoteCommandArg(op.Name)
	case AdoptVolume:
		return "garden adopt volume " + quoteCommandArg(op.Pool) + " " + quoteCommandArg(op.Name)
	case DeleteProject:
		return "garden delete project " + quoteCommandArg(op.Name)
	case DeleteVM:
		return "garden delete vm " + quoteCommandArg(op.Project) + " " + quoteCommandArg(op.Name)
	default:
		return "garden plan, then garden apply"
	}
}

func (op Operation) diagnostics(cause error) string {
	var commands []string
	project := op.Project
	if project == "" {
		project = "default"
	}
	scope := " --project " + quoteCommandArg(project)
	switch {
	case op.Pool != "":
		commands = append(commands, "incus storage volume show "+quoteCommandArg("garden:"+op.Pool)+" "+quoteCommandArg("custom/"+op.Name)+scope)
	case op.Project != "":
		commands = append(commands, "incus info "+quoteCommandArg("garden:"+op.Name)+" --show-log"+scope,
			"incus config show "+quoteCommandArg("garden:"+op.Name)+scope)
	default:
		commands = append(commands, "incus project show "+quoteCommandArg("garden:"+op.Name)+" --project 'default'")
	}
	var operation interface{ OperationID() string }
	if errors.As(cause, &operation) && operation.OperationID() != "" {
		commands = append(commands, "incus operation show "+quoteCommandArg("garden:"+operation.OperationID())+scope)
	}
	var out strings.Builder
	out.WriteString("\nInspect current state (a missing resource or expired operation may return not found):")
	for _, command := range commands {
		fmt.Fprintf(&out, "\n  %s", command)
	}
	fmt.Fprintf(&out, "\nResolve the cause and review current state before retrying: %s.", op.recovery())
	if op.Kind == DeleteProject || op.Kind == DeleteVM {
		out.WriteString(" Deletion requires confirmation again; if the resource is already absent, do not recreate it to retry.")
	}
	return out.String()
}

func (op Operation) failure(cause error, completed, total int) error {
	return fmt.Errorf("%s failed after %d completed operations (%d planned): %w\nLater operations were not attempted; completed changes were not rolled back.\nThe failed operation may have changed Incus state; its current outcome is unverified.%s",
		op.String(), completed, total, cause, op.diagnostics(cause))
}
