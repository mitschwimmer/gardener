package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

type lifecycleBackend struct {
	cliBackend
	snapshot reconcile.Snapshot
}

func (b *lifecycleBackend) Discover() (reconcile.Snapshot, error) { return b.snapshot, nil }

type brokenReader struct{ err error }

func (r brokenReader) Read([]byte) (int, error) { return 0, r.err }

func lifecycleCLI() *lifecycleBackend {
	return &lifecycleBackend{snapshot: reconcile.Snapshot{Projects: []api.Project{{Name: "apps", ProjectPut: api.ProjectPut{Config: reconcile.ProjectConfig()}}}}}
}

func TestDeleteCLIConfirmationAndIOFailures(t *testing.T) {
	for _, input := range []string{"", "yes\n", "other\n", strings.Repeat("a", 2048), "apps\n", "apps"} {
		b := lifecycleCLI()
		var output bytes.Buffer
		err := run([]string{"delete", "project", "apps"}, strings.NewReader(input), &output, io.Discard,
			definition.Garden{}, func() (reconcile.Backend, error) { return b, nil })
		confirmed := input == "apps\n" || input == "apps"
		if confirmed && (err != nil || b.executions != 1) || !confirmed && (err == nil || b.executions != 0) {
			t.Fatalf("input=%q error=%v writes=%d", input, err, b.executions)
		}
		if !strings.Contains(output.String(), `Type "apps" to confirm`) {
			t.Fatal("missing explicit target confirmation")
		}
	}
	want := errors.New("I/O unavailable")
	for _, brokenOutput := range []bool{false, true} {
		b := lifecycleCLI()
		var input io.Reader = brokenReader{want}
		var output io.Writer = io.Discard
		if brokenOutput {
			input, output = strings.NewReader("apps\n"), failingWriter{want}
		}
		err := run([]string{"delete", "project", "apps"}, input, output, io.Discard,
			definition.Garden{}, func() (reconcile.Backend, error) { return b, nil })
		if !errors.Is(err, want) || b.executions != 0 {
			t.Fatalf("I/O failure permitted mutation: %v writes=%d", err, b.executions)
		}
	}
}

func TestAdoptCLIAndMalformedLifecycleCommands(t *testing.T) {
	b := lifecycleCLI()
	delete(b.snapshot.Projects[0].Config, definition.OwnerKey)
	g := definition.Garden{Projects: []definition.Project{{Name: "apps"}}}
	err := run([]string{"adopt", "project", "apps"}, brokenReader{errors.New("must not read")}, io.Discard, io.Discard,
		g, func() (reconcile.Backend, error) { return b, nil })
	if err != nil || b.executions != 1 {
		t.Fatalf("adoption did not execute: %v", err)
	}
	for _, args := range [][]string{
		{"adopt"}, {"delete"}, {"delete", "project"}, {"adopt", "vm", "apps"},
		{"delete", "vm", "", "wiki"}, {"adopt", "project", "--yes"},
		{"delete", "project", "apps", "--yes"}, {"adopt", "container", "apps", "wiki"},
	} {
		err := run(args, strings.NewReader("apps\n"), io.Discard, io.Discard, g,
			func() (reconcile.Backend, error) {
				t.Fatalf("invalid arguments contacted Incus: %v", args)
				return nil, nil
			})
		if err == nil {
			t.Fatalf("accepted malformed lifecycle command: %v", args)
		}
	}
}
