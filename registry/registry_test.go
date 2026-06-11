package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupLibdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"stages", "sources", "devices", "inputs", "mounts", "runners", "schemas"} {
		os.MkdirAll(filepath.Join(dir, sub), 0o755)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestModuleUnknownKind(t *testing.T) {
	libdir := setupLibdir(t)
	reg := New(libdir)
	_, err := reg.Module("Bogus", "org.osbuild.test")
	if err == nil {
		t.Fatal("expected error for unknown module kind")
	}
}

func TestModuleMissingMeta(t *testing.T) {
	libdir := setupLibdir(t)
	writeFile(t, filepath.Join(libdir, "stages", "org.osbuild.test"), "#!/bin/bash\n")

	reg := New(libdir)
	m, err := reg.Module("Stage", "org.osbuild.test")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "org.osbuild.test" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Kind != "Stage" {
		t.Errorf("Kind = %q", m.Kind)
	}
	if len(m.Schemas) != 0 {
		t.Errorf("expected empty schemas, got %d", len(m.Schemas))
	}
	if len(m.Caps) != 0 {
		t.Errorf("expected empty caps, got %d", len(m.Caps))
	}
}

func TestModuleWithMeta(t *testing.T) {
	libdir := setupLibdir(t)
	writeFile(t, filepath.Join(libdir, "stages", "org.osbuild.rpm"), "#!/bin/bash\n")

	meta := map[string]interface{}{
		"summary": "Install RPM packages",
		"schema_2": map[string]interface{}{
			"options": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"packages": map[string]interface{}{"type": "array"},
				},
			},
		},
		"capabilities": []interface{}{"CAP_SYS_ADMIN"},
	}
	metaJSON, _ := json.Marshal(meta)
	writeFile(t, filepath.Join(libdir, "stages", "org.osbuild.rpm.meta.json"), string(metaJSON))

	reg := New(libdir)
	m, err := reg.Module("Stage", "org.osbuild.rpm")
	if err != nil {
		t.Fatal(err)
	}
	if m.Desc != "Install RPM packages" {
		t.Errorf("Desc = %q", m.Desc)
	}
	if _, ok := m.Schemas["2"]; !ok {
		t.Error("expected schema version 2")
	}
	if _, ok := m.Caps["CAP_SYS_ADMIN"]; !ok {
		t.Error("expected CAP_SYS_ADMIN capability")
	}
}

func TestModuleCaching(t *testing.T) {
	libdir := setupLibdir(t)
	writeFile(t, filepath.Join(libdir, "stages", "org.osbuild.test"), "")

	reg := New(libdir)
	m1, err := reg.Module("Stage", "org.osbuild.test")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := reg.Module("Stage", "org.osbuild.test")
	if err != nil {
		t.Fatal(err)
	}
	if m1 != m2 {
		t.Error("expected same pointer from cache")
	}
}

func TestBuildSchemaStage(t *testing.T) {
	m := &Module{
		Name: "org.osbuild.rpm",
		Kind: "Stage",
		Schemas: map[string]interface{}{
			"2": map[string]interface{}{
				"options": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"packages": map[string]interface{}{"type": "array"},
					},
				},
			},
		},
	}

	schema := m.BuildSchema("2")
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}

	typeEnum, ok := props["type"].(map[string]interface{})
	if !ok {
		t.Fatal("expected type property")
	}
	enum := typeEnum["enum"].([]interface{})
	if len(enum) != 1 || enum[0] != "org.osbuild.rpm" {
		t.Errorf("type enum = %v", enum)
	}

	if _, ok := props["mounts"]; !ok {
		t.Error("expected mounts property")
	}
	if _, ok := props["devices"]; !ok {
		t.Error("expected devices property")
	}
	if _, ok := props["options"]; !ok {
		t.Error("expected options property")
	}
}

func TestBuildSchemaDevice(t *testing.T) {
	m := &Module{
		Name: "org.osbuild.loopback",
		Kind: "Device",
		Schemas: map[string]interface{}{
			"2": map[string]interface{}{
				"options": map[string]interface{}{
					"type": "object",
				},
			},
		},
	}

	schema := m.BuildSchema("2")
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties")
	}
	typeEnum, ok := props["type"].(map[string]interface{})
	if !ok {
		t.Fatal("expected type property")
	}
	enum := typeEnum["enum"].([]interface{})
	if len(enum) != 1 || enum[0] != "org.osbuild.loopback" {
		t.Errorf("type enum = %v", enum)
	}
}

func TestBuildSchemaMount(t *testing.T) {
	m := &Module{
		Name: "org.osbuild.ext4",
		Kind: "Mount",
		Schemas: map[string]interface{}{
			"2": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target": map[string]interface{}{"type": "string"},
				},
			},
		},
	}

	schema := m.BuildSchema("2")
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties")
	}
	typeEnum, ok := props["type"].(map[string]interface{})
	if !ok {
		t.Fatal("expected type property injected")
	}
	enum := typeEnum["enum"].([]interface{})
	if enum[0] != "org.osbuild.ext4" {
		t.Errorf("type enum = %v", enum)
	}
	if _, ok := props["target"]; !ok {
		t.Error("original target property should be preserved")
	}
}

func TestBuildSchemaGeneric(t *testing.T) {
	m := &Module{
		Name: "org.osbuild.curl",
		Kind: "Source",
		Schemas: map[string]interface{}{
			"2": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"items": map[string]interface{}{"type": "object"},
				},
			},
		},
	}

	schema := m.BuildSchema("2")
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}
	if _, ok := schema["properties"]; !ok {
		t.Error("expected properties copied")
	}
}

func TestBuildSchemaNil(t *testing.T) {
	m := &Module{Name: "test", Kind: "Stage", Schemas: map[string]interface{}{}}
	schema := m.BuildSchema("2")
	if schema != nil {
		t.Error("expected nil for missing version")
	}
}

func TestSchemaLookupModule(t *testing.T) {
	libdir := setupLibdir(t)
	writeFile(t, filepath.Join(libdir, "stages", "org.osbuild.test"), "")
	meta := map[string]interface{}{
		"schema_2": map[string]interface{}{
			"options": map[string]interface{}{"type": "object"},
		},
	}
	metaJSON, _ := json.Marshal(meta)
	writeFile(t, filepath.Join(libdir, "stages", "org.osbuild.test.meta.json"), string(metaJSON))

	reg := New(libdir)
	s := reg.Schema("Stage", "org.osbuild.test", "2")
	if s.Data == nil {
		t.Fatal("expected schema data")
	}
	if _, ok := s.Data["properties"]; !ok {
		t.Error("expected built schema to have properties")
	}
}

func TestSchemaLookupManifest(t *testing.T) {
	libdir := setupLibdir(t)
	schemaData := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"version": map[string]interface{}{"type": "string"},
		},
	}
	data, _ := json.Marshal(schemaData)
	writeFile(t, filepath.Join(libdir, "schemas", "osbuild2.json"), string(data))

	reg := New(libdir)
	s := reg.Schema("Manifest", "", "2")
	if s.Data == nil {
		t.Fatal("expected manifest schema data")
	}
}

func TestSchemaCaching(t *testing.T) {
	libdir := setupLibdir(t)
	writeFile(t, filepath.Join(libdir, "stages", "org.osbuild.test"), "")

	reg := New(libdir)
	s1 := reg.Schema("Stage", "org.osbuild.test", "2")
	s2 := reg.Schema("Stage", "org.osbuild.test", "2")
	if s1 != s2 {
		t.Error("expected same pointer from cache")
	}
}

func TestListRunners(t *testing.T) {
	libdir := setupLibdir(t)
	for _, name := range []string{"org.osbuild.fedora39", "org.osbuild.fedora40", "org.osbuild.rhel9"} {
		writeFile(t, filepath.Join(libdir, "runners", name), "")
	}

	reg := New(libdir)
	all := reg.ListRunners("")
	if len(all) != 3 {
		t.Fatalf("expected 3 runners, got %d", len(all))
	}

	fedora := reg.ListRunners("org.osbuild.fedora")
	if len(fedora) != 2 {
		t.Fatalf("expected 2 fedora runners, got %d", len(fedora))
	}
	for _, r := range fedora {
		if r.Distro != "org.osbuild.fedora" {
			t.Errorf("unexpected distro: %s", r.Distro)
		}
	}
}

func TestDetectRunner(t *testing.T) {
	libdir := setupLibdir(t)
	for _, name := range []string{"org.osbuild.fedora38", "org.osbuild.fedora39", "org.osbuild.fedora40"} {
		writeFile(t, filepath.Join(libdir, "runners", name), "")
	}

	reg := New(libdir)

	ri, err := reg.DetectRunner("org.osbuild.fedora40")
	if err != nil {
		t.Fatal(err)
	}
	if ri.Version != 40 {
		t.Errorf("version = %d, want 40", ri.Version)
	}

	ri, err = reg.DetectRunner("org.osbuild.fedora41")
	if err != nil {
		t.Fatal(err)
	}
	if ri.Version != 40 {
		t.Errorf("version = %d, want 40 (highest ≤ 41)", ri.Version)
	}
}

func TestDetectRunnerNotFound(t *testing.T) {
	libdir := setupLibdir(t)
	writeFile(t, filepath.Join(libdir, "runners", "org.osbuild.fedora40"), "")

	reg := New(libdir)
	_, err := reg.DetectRunner("org.osbuild.rhel9")
	if err == nil {
		t.Fatal("expected error for missing distro")
	}
}

func TestParseRunnerName(t *testing.T) {
	tests := []struct {
		name    string
		distro  string
		version int
	}{
		{"org.osbuild.fedora40", "org.osbuild.fedora", 40},
		{"org.osbuild.rhel9", "org.osbuild.rhel", 9},
		{"org.osbuild.centos", "org.osbuild.centos", 0},
		{"123", "", 123},
	}

	for _, tt := range tests {
		distro, version := parseRunnerName(tt.name)
		if distro != tt.distro || version != tt.version {
			t.Errorf("parseRunnerName(%q) = (%q, %d), want (%q, %d)",
				tt.name, distro, version, tt.distro, tt.version)
		}
	}
}
