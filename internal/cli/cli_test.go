package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

func TestHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run([]string{arg}, strings.NewReader(""), &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage: garden") {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestUnimplementedCommandsFail(t *testing.T) {
	for _, command := range []string{"adopt", "delete", "unknown"} {
		t.Run(command, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := Run([]string{command}, strings.NewReader(""), &stdout, io.Discard); err == nil {
				t.Fatal("expected an error")
			}
			if stdout.Len() != 0 {
				t.Fatalf("unexpected success output: %q", stdout.String())
			}
		})
	}
}

func TestUnknownFlagFails(t *testing.T) {
	if err := Run([]string{"--remote=other"}, strings.NewReader(""), io.Discard, io.Discard); err == nil {
		t.Fatal("expected an error")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestOutputFailure(t *testing.T) {
	want := errors.New("output unavailable")
	if err := Run(nil, strings.NewReader(""), failingWriter{want}, io.Discard); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

type cliBackend struct{ executions, discovered int }

func (b *cliBackend) Discover() (reconcile.Snapshot, error) {
	b.discovered++
	return reconcile.Snapshot{}, nil
}
func (b *cliBackend) CheckReferences(definition.VM) error { return nil }
func (b *cliBackend) Execute(reconcile.Operation) error   { b.executions++; return nil }

func TestPlanAndApply(t *testing.T) {
	for _, command := range []string{"plan", "apply"} {
		b := &cliBackend{}
		var output bytes.Buffer
		g := definition.Garden{Projects: []definition.Project{{Name: "apps"}}}
		err := run([]string{command}, strings.NewReader(""), &output, io.Discard, g, func() (reconcile.Backend, error) { return b, nil })
		if err != nil {
			t.Fatal(err)
		}
		if b.discovered != 1 || !strings.Contains(output.String(), "create project apps") {
			t.Fatalf("missing fresh discovery or summary: %s", output.String())
		}
		if command == "plan" && b.executions != 0 {
			t.Fatal("plan performed a mutation")
		}
		if command == "apply" && b.executions != 1 {
			t.Fatal("apply did not execute")
		}
	}
}

func TestInvalidDefinitionDoesNotConnect(t *testing.T) {
	connected := false
	g := definition.Garden{Projects: []definition.Project{{Name: "default"}}}
	err := run([]string{"apply"}, strings.NewReader(""), io.Discard, io.Discard, g, func() (reconcile.Backend, error) { connected = true; return &cliBackend{}, nil })
	if err == nil || connected {
		t.Fatal("invalid definition contacted Incus")
	}
}

func TestConnectionErrorAndExtraArguments(t *testing.T) {
	want := errors.New("connection unavailable")
	err := run([]string{"plan"}, strings.NewReader(""), io.Discard, io.Discard, definition.Garden{}, func() (reconcile.Backend, error) { return nil, want })
	if !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
	for _, args := range [][]string{{"apply", "extra"}, {"plan", "--remote=other"}} {
		err := run(args, strings.NewReader(""), io.Discard, io.Discard, definition.Garden{}, func() (reconcile.Backend, error) { t.Fatal("unexpected connection"); return nil, nil })
		if err == nil {
			t.Fatalf("accepted unsupported arguments %v", args)
		}
	}
}
