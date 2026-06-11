package manifest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/supakeen/image-assembler/registry"
)

func TestFormatValidationErrorsValid(t *testing.T) {
	r := &registry.ValidationResult{Origin: "test"}
	got := FormatValidationErrors(r)
	if got != "" {
		t.Errorf("valid result should format as empty, got %q", got)
	}
}

func TestFormatValidationErrorsWithErrors(t *testing.T) {
	r := &registry.ValidationResult{
		Origin: "test",
		Errors: []registry.ValidationError{
			{Message: "missing field", Path: []interface{}{"pipelines", 0, "stages", 1}},
			{Message: "wrong type"},
		},
	}

	got := FormatValidationErrors(r)
	if !strings.Contains(got, "Validation failed") {
		t.Error("expected 'Validation failed' header")
	}
	if !strings.Contains(got, ".pipelines[0].stages[1]: missing field") {
		t.Errorf("expected path-prefixed error, got:\n%s", got)
	}
	if !strings.Contains(got, "wrong type") {
		t.Errorf("expected pathless error, got:\n%s", got)
	}
}

func setupValidationLibdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"stages", "sources", "schemas"} {
		os.MkdirAll(filepath.Join(dir, sub), 0o755)
	}

	stageSchema := map[string]interface{}{
		"options": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"packages": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
			},
			"required": []interface{}{"packages"},
		},
	}

	meta := map[string]interface{}{
		"summary":  "Install packages",
		"schema_2": stageSchema,
	}
	metaJSON, _ := json.Marshal(meta)
	writeTestFile(t, filepath.Join(dir, "stages", "org.osbuild.rpm"), "")
	writeTestFile(t, filepath.Join(dir, "stages", "org.osbuild.rpm.meta.json"), string(metaJSON))

	return dir
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateValidManifest(t *testing.T) {
	libdir := setupValidationLibdir(t)
	reg := registry.New(libdir)

	desc := map[string]interface{}{
		"version": "2",
		"pipelines": []interface{}{
			map[string]interface{}{
				"name": "build",
				"stages": []interface{}{
					map[string]interface{}{
						"type": "org.osbuild.rpm",
						"options": map[string]interface{}{
							"packages": []interface{}{"vim", "bash"},
						},
					},
				},
			},
		},
	}

	r := Validate(context.Background(), desc, reg)
	if !r.Valid() {
		t.Errorf("expected valid, got errors: %v", r.Errors)
	}
}

func TestValidateInvalidStageOptions(t *testing.T) {
	libdir := setupValidationLibdir(t)
	reg := registry.New(libdir)

	desc := map[string]interface{}{
		"version": "2",
		"pipelines": []interface{}{
			map[string]interface{}{
				"name": "build",
				"stages": []interface{}{
					map[string]interface{}{
						"type": "org.osbuild.rpm",
						"options": map[string]interface{}{
							"bogus_field": "not allowed",
						},
					},
				},
			},
		},
	}

	r := Validate(context.Background(), desc, reg)
	if r.Valid() {
		t.Error("expected validation errors for invalid options")
	}
}

func TestValidateNoStages(t *testing.T) {
	libdir := setupValidationLibdir(t)
	reg := registry.New(libdir)

	desc := map[string]interface{}{
		"version":   "2",
		"pipelines": []interface{}{},
	}

	r := Validate(context.Background(), desc, reg)
	if !r.Valid() {
		t.Errorf("empty pipelines should be valid, got: %v", r.Errors)
	}
}

func TestValidateCancelledContext(t *testing.T) {
	libdir := setupValidationLibdir(t)
	reg := registry.New(libdir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	desc := map[string]interface{}{
		"version": "2",
		"sources": map[string]interface{}{
			"org.osbuild.curl": map[string]interface{}{
				"items": map[string]interface{}{"sha256:abc": map[string]interface{}{}},
			},
		},
	}

	r := Validate(ctx, desc, reg)
	if r.Valid() {
		t.Error("cancelled context should produce errors")
	}
}
