package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run([]string{arg}, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage: garden") {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestUnimplementedCommandsFail(t *testing.T) {
	for _, command := range []string{"apply", "adopt", "delete", "unknown"} {
		t.Run(command, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := Run([]string{command}, &stdout, io.Discard); err == nil {
				t.Fatal("expected an error")
			}
			if stdout.Len() != 0 {
				t.Fatalf("unexpected success output: %q", stdout.String())
			}
		})
	}
}

func TestUnknownFlagFails(t *testing.T) {
	if err := Run([]string{"--remote=other"}, io.Discard, io.Discard); err == nil {
		t.Fatal("expected an error")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestOutputFailure(t *testing.T) {
	want := errors.New("output unavailable")
	if err := Run(nil, failingWriter{want}, io.Discard); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}
