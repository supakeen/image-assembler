package sandbox

import (
	"testing"
)

func TestBuildCapArgsNil(t *testing.T) {
	args := BuildCapArgs(nil)
	if args != nil {
		t.Errorf("expected nil for nil caps, got %v", args)
	}
}

func TestBuildCapArgsEmpty(t *testing.T) {
	args := BuildCapArgs(map[string]struct{}{})
	if len(args) != 2 || args[0] != "--cap-drop" || args[1] != "ALL" {
		t.Errorf("expected [--cap-drop ALL], got %v", args)
	}
}

func TestBuildCapArgsSorted(t *testing.T) {
	caps := map[string]struct{}{
		"CAP_CHOWN":    {},
		"CAP_SETUID":   {},
		"CAP_DAC_OVERRIDE": {},
	}

	args := BuildCapArgs(caps)

	expected := []string{
		"--cap-drop", "ALL",
		"--cap-add", "CAP_CHOWN",
		"--cap-add", "CAP_DAC_OVERRIDE",
		"--cap-add", "CAP_SETUID",
	}

	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, v := range expected {
		if args[i] != v {
			t.Errorf("args[%d] = %q, want %q", i, args[i], v)
		}
	}
}

func TestDefaultCapsCount(t *testing.T) {
	if len(DefaultCaps) != 19 {
		t.Errorf("expected 19 default caps, got %d", len(DefaultCaps))
	}
}
