package reconcile

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type diagnosticFailure struct{ cause error }

func (e diagnosticFailure) Error() string       { return "operation failed: " + e.cause.Error() }
func (e diagnosticFailure) Unwrap() error       { return e.cause }
func (e diagnosticFailure) OperationID() string { return "operation-id" }

func TestFailureContextAndRecoveryCommands(t *testing.T) {
	cause := errors.New("lost connection")
	for _, tc := range []struct {
		op             Operation
		inspect, retry string
	}{
		{Operation{Kind: StartVM, Project: "apps", Name: "wiki"}, "incus info 'garden:wiki' --project 'apps'", "garden plan, then garden apply"},
		{Operation{Kind: AdoptVM, Project: "apps", Name: "wiki"}, "incus config show 'garden:wiki' --project 'apps'", "garden adopt vm 'apps' 'wiki'"},
		{Operation{Kind: DeleteVM, Project: "apps", Name: "wiki"}, "incus info 'garden:wiki'", "garden delete vm 'apps' 'wiki'"},
		{Operation{Kind: AdoptProject, Name: "apps"}, "incus project show 'garden:apps' --project 'default'", "garden adopt project 'apps'"},
		{Operation{Kind: DeleteProject, Name: "apps"}, "incus project show 'garden:apps'", "garden delete project 'apps'"},
		{Operation{Kind: AdoptVolume, Pool: "pool", Name: "data"}, "incus storage volume show 'garden:pool' 'custom/data' --project 'default'", "garden adopt volume 'pool' 'data'"},
	} {
		err := tc.op.failure(diagnosticFailure{cause}, 2, 5)
		if !errors.Is(err, cause) {
			t.Fatal("lost error chain")
		}
		for _, want := range []string{tc.inspect, tc.retry, "after 2 completed operations (5 planned)", "current outcome is unverified", "not rolled back", "incus operation show 'garden:operation-id'"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("missing %q in %s", want, err)
			}
		}
		if tc.op.Kind == DeleteVM || tc.op.Kind == DeleteProject {
			if !strings.Contains(err.Error(), "requires confirmation again") {
				t.Fatal(err)
			}
		}
	}
}

type failOutput struct {
	prefix string
	bytes.Buffer
}

func (w *failOutput) Write(p []byte) (int, error) {
	if strings.HasPrefix(string(p), w.prefix) {
		return 0, io.ErrClosedPipe
	}
	return w.Buffer.Write(p)
}

func TestProgressAndReportingFailuresStopSafely(t *testing.T) {
	for _, tc := range []struct {
		prefix  string
		writes  int
		message string
	}{
		{"Executing", 0, "this operation was not attempted"},
		{"Completed", 1, "create project apps completed successfully"},
		{"Apply complete", 2, "all 2 operations completed successfully"},
	} {
		f := emptyBackend()
		p := Plan{Operations: []Operation{{Kind: CreateProject, Name: "apps"}, {Kind: CreateProject, Name: "other"}}}
		w := &failOutput{prefix: tc.prefix}
		err := p.Apply(f, w)
		if !errors.Is(err, io.ErrClosedPipe) || len(f.executed) != tc.writes || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("%s: err=%v writes=%d", tc.prefix, err, len(f.executed))
		}
	}
	f := emptyBackend()
	p, err := Build(desired(), f)
	if err != nil {
		t.Fatal(err)
	}
	f.failAt = 2
	var out bytes.Buffer
	err = p.Apply(f, &out)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(out.String(), "Executing [2/3]: create VM apps/wiki") || strings.Contains(out.String(), "Executing [3/3]") {
		t.Fatal(out.String())
	}
}

func TestDiagnosticCommandsQuoteNames(t *testing.T) {
	op := Operation{Kind: AdoptVolume, Pool: "pool", Name: "a'$(touch /tmp/unwanted)"}
	out := op.diagnostics(errors.New("failed"))
	if !strings.Contains(out, "'custom/a'\\''$(touch /tmp/unwanted)'") || !strings.Contains(out, "'a'\\''$(touch /tmp/unwanted)'") {
		t.Fatal(out)
	}
	if strings.Contains(out, "incus operation show") {
		t.Fatal("invented operation ID")
	}
}
