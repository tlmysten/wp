package ui

import (
	"bytes"
	"os"
	"testing"
)

func TestTagDoesNotColorNonTerminalWriter(t *testing.T) {
	var output bytes.Buffer
	if got := Tag(&output, "OK"); got != "OK    " {
		t.Fatalf("tag = %q, want %q", got, "OK    ")
	}
}

func TestForceColorEnablesColor(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")
	if got := Tag(&bytes.Buffer{}, "ERR"); got != "\x1b[31mERR   \x1b[0m" {
		t.Fatalf("tag = %q", got)
	}
}

func TestNoColorOverridesForceColor(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "1")
	if got := Tag(os.Stdout, "READY"); got != "READY " {
		t.Fatalf("tag = %q, want %q", got, "READY ")
	}
}
