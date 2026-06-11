package build

import (
	"testing"

	"github.com/supakeen/image-assembler/manifest"
)

func TestDeviceManagerRelPathNil(t *testing.T) {
	dm := NewDeviceManager(nil, "/dev", "/tree")
	if got := dm.RelPath(nil); got != "" {
		t.Errorf("RelPath(nil) = %q, want empty", got)
	}
}

func TestDeviceManagerRelPathKnown(t *testing.T) {
	dm := NewDeviceManager(nil, "/dev", "/tree")
	dm.devices["disk"] = map[string]interface{}{"path": "loop0"}

	dev := &manifest.Device{Name: "disk"}
	if got := dm.RelPath(dev); got != "loop0" {
		t.Errorf("RelPath = %q, want loop0", got)
	}
}

func TestDeviceManagerRelPathUnknown(t *testing.T) {
	dm := NewDeviceManager(nil, "/dev", "/tree")
	dev := &manifest.Device{Name: "unknown"}
	if got := dm.RelPath(dev); got != "" {
		t.Errorf("RelPath(unknown) = %q, want empty", got)
	}
}

func TestDeviceManagerAbsPath(t *testing.T) {
	dm := NewDeviceManager(nil, "/run/osbuild/dev-xyz", "/tree")
	dm.devices["disk"] = map[string]interface{}{"path": "loop7"}

	dev := &manifest.Device{Name: "disk"}
	want := "/run/osbuild/dev-xyz/loop7"
	if got := dm.AbsPath(dev); got != want {
		t.Errorf("AbsPath = %q, want %q", got, want)
	}
}

func TestDeviceManagerAbsPathNil(t *testing.T) {
	dm := NewDeviceManager(nil, "/dev", "/tree")
	if got := dm.AbsPath(nil); got != "" {
		t.Errorf("AbsPath(nil) = %q, want empty", got)
	}
}
