package manifest

import (
	"testing"
)

func makeStage(typ string, options map[string]interface{}) Stage {
	if options == nil {
		options = make(map[string]interface{})
	}
	return Stage{
		Type:    typ,
		Options: options,
		Inputs:  NewOrderedMap[*Input](),
		Devices: NewOrderedMap[*Device](),
		Mounts:  NewOrderedMap[*Mount](),
	}
}

func TestStageIDDeterministic(t *testing.T) {
	s1 := makeStage("org.osbuild.rpm", map[string]interface{}{"packages": []interface{}{"vim"}})
	s2 := makeStage("org.osbuild.rpm", map[string]interface{}{"packages": []interface{}{"vim"}})

	if s1.ID() != s2.ID() {
		t.Errorf("same stage should produce same ID: %s vs %s", s1.ID(), s2.ID())
	}
}

func TestStageIDChangesWithOptions(t *testing.T) {
	s1 := makeStage("org.osbuild.rpm", map[string]interface{}{"packages": []interface{}{"vim"}})
	s2 := makeStage("org.osbuild.rpm", map[string]interface{}{"packages": []interface{}{"emacs"}})

	if s1.ID() == s2.ID() {
		t.Error("different options should produce different IDs")
	}
}

func TestStageIDChangesWithType(t *testing.T) {
	s1 := makeStage("org.osbuild.rpm", nil)
	s2 := makeStage("org.osbuild.copy", nil)

	if s1.ID() == s2.ID() {
		t.Error("different types should produce different IDs")
	}
}

func TestStageIDWithInputs(t *testing.T) {
	s1 := makeStage("org.osbuild.rpm", nil)

	s2 := makeStage("org.osbuild.rpm", nil)
	s2.Inputs.Set("packages", &Input{
		Type:    "org.osbuild.files",
		Origin:  "org.osbuild.source",
		Refs:    map[string]interface{}{"sha256:abc": map[string]interface{}{}},
		Options: map[string]interface{}{},
	})

	if s1.ID() == s2.ID() {
		t.Error("stage with inputs should have different ID than without")
	}
}

func TestStageIDWithMounts(t *testing.T) {
	s1 := makeStage("org.osbuild.rpm", nil)

	s2 := makeStage("org.osbuild.rpm", nil)
	s2.Mounts.Set("root", &Mount{
		Type:    "org.osbuild.ext4",
		Target:  "/",
		Options: map[string]interface{}{},
	})

	if s1.ID() == s2.ID() {
		t.Error("stage with mounts should have different ID than without")
	}
}

func TestStageIDWithSourceEpoch(t *testing.T) {
	s1 := makeStage("org.osbuild.rpm", nil)

	epoch := 1700000000
	s2 := makeStage("org.osbuild.rpm", nil)
	s2.SourceEpoch = &epoch

	if s1.ID() == s2.ID() {
		t.Error("stage with source epoch should have different ID than without")
	}
}

func TestStageIDWithBuildRef(t *testing.T) {
	s1 := makeStage("org.osbuild.rpm", nil)

	buildRef := "abc123"
	s2 := makeStage("org.osbuild.rpm", nil)
	s2.Build = &buildRef

	if s1.ID() == s2.ID() {
		t.Error("stage with build ref should have different ID than without")
	}
}

func TestInputID(t *testing.T) {
	i1 := &Input{
		Type:    "org.osbuild.files",
		Origin:  "org.osbuild.source",
		Refs:    map[string]interface{}{"sha256:abc": map[string]interface{}{}},
		Options: map[string]interface{}{},
	}
	i2 := &Input{
		Type:    "org.osbuild.files",
		Origin:  "org.osbuild.source",
		Refs:    map[string]interface{}{"sha256:abc": map[string]interface{}{}},
		Options: map[string]interface{}{},
	}
	if i1.ID() != i2.ID() {
		t.Error("same input should produce same ID")
	}

	i3 := &Input{
		Type:    "org.osbuild.files",
		Origin:  "org.osbuild.source",
		Refs:    map[string]interface{}{"sha256:def": map[string]interface{}{}},
		Options: map[string]interface{}{},
	}
	if i1.ID() == i3.ID() {
		t.Error("different refs should produce different IDs")
	}
}

func TestDeviceID(t *testing.T) {
	d1 := &Device{
		Type:    "org.osbuild.loopback",
		Options: map[string]interface{}{"filename": "disk.img"},
	}
	d2 := &Device{
		Type:    "org.osbuild.loopback",
		Options: map[string]interface{}{"filename": "disk.img"},
	}
	if d1.ID() != d2.ID() {
		t.Error("same device should produce same ID")
	}

	d3 := &Device{
		Type:    "org.osbuild.loopback",
		Parent:  d1,
		Options: map[string]interface{}{"filename": "disk.img"},
	}
	if d1.ID() == d3.ID() {
		t.Error("device with parent should have different ID")
	}
}

func TestDeviceIDParentChain(t *testing.T) {
	parent := &Device{
		Type:    "org.osbuild.loopback",
		Options: map[string]interface{}{"filename": "disk.img"},
	}
	child1 := &Device{
		Type:    "org.osbuild.loopback",
		Parent:  parent,
		Options: map[string]interface{}{"partition": float64(1)},
	}
	child2 := &Device{
		Type:    "org.osbuild.loopback",
		Parent:  parent,
		Options: map[string]interface{}{"partition": float64(2)},
	}

	if child1.ID() == child2.ID() {
		t.Error("different children should have different IDs")
	}

	parentID := parent.ID()
	if parentID == child1.ID() {
		t.Error("parent and child should have different IDs")
	}
}

func TestMountID(t *testing.T) {
	m1 := &Mount{
		Type:    "org.osbuild.ext4",
		Target:  "/",
		Options: map[string]interface{}{},
	}
	m2 := &Mount{
		Type:    "org.osbuild.ext4",
		Target:  "/boot",
		Options: map[string]interface{}{},
	}
	if m1.ID() == m2.ID() {
		t.Error("different targets should produce different IDs")
	}

	part := 1
	m3 := &Mount{
		Type:      "org.osbuild.ext4",
		Target:    "/",
		Partition: &part,
		Options:   map[string]interface{}{},
	}
	if m1.ID() == m3.ID() {
		t.Error("with vs without partition should produce different IDs")
	}

	dev := &Device{Type: "org.osbuild.loopback", Options: map[string]interface{}{}}
	m4 := &Mount{
		Type:    "org.osbuild.ext4",
		Target:  "/",
		Device:  dev,
		Options: map[string]interface{}{},
	}
	if m1.ID() == m4.ID() {
		t.Error("with vs without device should produce different IDs")
	}
}

func TestSourceID(t *testing.T) {
	s1 := &Source{
		Type:  "org.osbuild.curl",
		Items: map[string]interface{}{"sha256:abc": map[string]interface{}{"url": "https://example.com"}},
	}
	s2 := &Source{
		Type:  "org.osbuild.curl",
		Items: map[string]interface{}{"sha256:abc": map[string]interface{}{"url": "https://example.com"}},
	}
	if s1.ID() != s2.ID() {
		t.Error("same source should produce same ID")
	}

	s3 := &Source{
		Type:  "org.osbuild.curl",
		Items: map[string]interface{}{"sha256:def": map[string]interface{}{"url": "https://other.com"}},
	}
	if s1.ID() == s3.ID() {
		t.Error("different items should produce different IDs")
	}
}
