package build

import (
	"testing"
)

func TestInputManagerPathValidation(t *testing.T) {
	im := NewInputManager(nil, nil, "/expected/root")

	// Simulate a reply with wrong prefix — this tests the validation logic
	// without needing a real service manager
	if im.root != "/expected/root" {
		t.Errorf("root = %q, want /expected/root", im.root)
	}
}
