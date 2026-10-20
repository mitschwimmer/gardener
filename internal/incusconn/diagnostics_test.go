package incusconn

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mitschwimmer/gardener/internal/reconcile"
)

func TestWaitErrorRetainsOperationAndCause(t *testing.T) {
	cause := errors.New("lost response")
	op := &recordingOperation{err: cause}
	err := wait(op, nil)
	var detail interface{ OperationID() string }
	if !errors.Is(err, cause) || !errors.As(err, &detail) || detail.OperationID() != "test-operation" {
		t.Fatal(err)
	}
	if err := wait(nil, cause); err != cause {
		t.Fatal("request failure attempted operation lookup", err)
	}
}

func TestReconfigurationReportsConfirmedStepsAndUncertainOutcome(t *testing.T) {
	phases := []string{"record pending power intent", "stop VM", "update VM", "start VM", "clear pending power intent"}
	states := []string{"Running", "Running", "Stopped", "Stopped", "Running"}
	for _, lostResponse := range []bool{false, true} {
		for failAt := 1; failAt <= 5; failAt++ {
			f, g := reconfigurationFixture()
			f.failAt = failAt
			f.failAfter = lostResponse
			b := Backend{Server: f}
			plan, err := reconcile.Build(g, b)
			if err != nil {
				t.Fatal(err)
			}
			err = plan.Apply(b, io.Discard)
			if err == nil {
				t.Fatal("expected failure")
			}
			pending := "running"
			if failAt == 1 {
				pending = "none"
			}
			confirmed := strings.Join(phases[:failAt-1], ", ")
			if confirmed == "" {
				confirmed = "none"
			}
			for _, want := range []string{
				"failed after 0 completed operations (1 planned)",
				phases[failAt-1] + ":",
				"Confirmed substeps within this reconfiguration: " + confirmed + ".",
				"Last observed VM state (before failure, current state unverified): power=" + states[failAt-1] + ", pending-power=" + pending,
				"incus operation show 'garden:test-reconfiguration' --project 'apps'",
				"incus config show 'garden:wiki' --project 'apps'",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("phase=%d lost=%v missing %q: %s", failAt, lostResponse, want, err)
				}
			}
		}
	}
}
