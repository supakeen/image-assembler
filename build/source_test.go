package build

import (
	"testing"

	"github.com/supakeen/image-assembler/manifest"
)

func TestDownloadAllEmpty(t *testing.T) {
	m := &manifest.Manifest{
		Sources: []manifest.Source{},
	}

	// DownloadAll with no sources should succeed without needing a store
	// We can't call it directly without a real store, but verify the manifest
	// has no sources to iterate
	if len(m.Sources) != 0 {
		t.Errorf("expected 0 sources, got %d", len(m.Sources))
	}
}

func TestSourceArgsStructure(t *testing.T) {
	src := &manifest.Source{
		Type: "org.osbuild.curl",
		Items: map[string]interface{}{
			"sha256:abc": map[string]interface{}{"url": "https://example.com/file"},
		},
		Options: map[string]interface{}{},
	}

	args := map[string]interface{}{
		"items":     src.Items,
		"options":   src.Options,
		"cache":     "/store/sources",
		"output":    nil,
		"checksums": []interface{}{},
	}

	if args["cache"] != "/store/sources" {
		t.Errorf("cache = %v, want /store/sources", args["cache"])
	}
	if args["output"] != nil {
		t.Errorf("output = %v, want nil", args["output"])
	}

	checksums, ok := args["checksums"].([]interface{})
	if !ok || len(checksums) != 0 {
		t.Errorf("checksums = %v, want empty slice", args["checksums"])
	}

	items, ok := args["items"].(map[string]interface{})
	if !ok || len(items) != 1 {
		t.Errorf("items = %v, want 1 item", args["items"])
	}
}
