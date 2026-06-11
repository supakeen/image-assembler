package build

import (
	"fmt"
	"testing"

	"github.com/supakeen/image-assembler/manifest"
)

func TestMountPartitionSuffix(t *testing.T) {
	dm := NewDeviceManager(nil, "/dev", "/tree")
	dm.devices["disk"] = map[string]interface{}{"path": "loop0"}

	dev := &manifest.Device{Name: "disk"}
	partition := 1
	mnt := &manifest.Mount{
		Name:      "root",
		Device:    dev,
		Partition: &partition,
		Target:    "/",
	}

	source := dm.AbsPath(mnt.Device)
	if mnt.Partition != nil {
		source = fmt.Sprintf("%sp%d", source, *mnt.Partition)
	}

	want := "/dev/loop0p1"
	if source != want {
		t.Errorf("source = %q, want %q", source, want)
	}
}

func TestMountNoPartition(t *testing.T) {
	dm := NewDeviceManager(nil, "/dev", "/tree")
	dm.devices["disk"] = map[string]interface{}{"path": "loop5"}

	dev := &manifest.Device{Name: "disk"}
	mnt := &manifest.Mount{
		Name:   "root",
		Device: dev,
		Target: "/",
	}

	source := dm.AbsPath(mnt.Device)
	if mnt.Partition != nil {
		source = fmt.Sprintf("%sp%d", source, *mnt.Partition)
	}

	want := "/dev/loop5"
	if source != want {
		t.Errorf("source = %q, want %q", source, want)
	}
}

func TestMountNilDevice(t *testing.T) {
	dm := NewDeviceManager(nil, "/dev", "/tree")

	mnt := &manifest.Mount{
		Name:   "root",
		Target: "/",
	}

	source := dm.AbsPath(mnt.Device)
	if source != "" {
		t.Errorf("source = %q, want empty", source)
	}
}
