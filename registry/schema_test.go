package registry

import (
	"testing"
)

func TestValidateSimpleSchemaValid(t *testing.T) {
	s := Schema{
		Data: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"name"},
		},
		Name: "test",
	}

	target := map[string]interface{}{"name": "hello"}
	r := s.Validate(target)
	if !r.Valid() {
		t.Errorf("expected valid, got errors: %v", r.Errors)
	}
}

func TestValidateSimpleSchemaInvalid(t *testing.T) {
	s := Schema{
		Data: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"name"},
		},
		Name: "test",
	}

	target := map[string]interface{}{"name": 42}
	r := s.Validate(target)
	if r.Valid() {
		t.Error("expected validation errors for wrong type")
	}
}

func TestValidateSimpleSchemaMissingRequired(t *testing.T) {
	s := Schema{
		Data: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"name"},
		},
		Name: "test",
	}

	target := map[string]interface{}{}
	r := s.Validate(target)
	if r.Valid() {
		t.Error("expected validation errors for missing required field")
	}
}

func TestValidateNilSchema(t *testing.T) {
	s := Schema{Data: nil, Name: "empty"}
	r := s.Validate(map[string]interface{}{"anything": true})
	if !r.Valid() {
		t.Errorf("nil schema should validate anything, got errors: %v", r.Errors)
	}
}

func TestValidationResultValid(t *testing.T) {
	r := &ValidationResult{Origin: "test"}
	if !r.Valid() {
		t.Error("empty result should be valid")
	}
}

func TestValidationResultMerge(t *testing.T) {
	r := &ValidationResult{Origin: "test"}
	other := &ValidationResult{
		Errors: []ValidationError{
			{Message: "bad field", Path: []interface{}{"options", "key"}},
		},
	}

	r.Merge(other, "pipelines", 0, "stages", 1)

	if len(r.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(r.Errors))
	}

	e := r.Errors[0]
	if e.Message != "bad field" {
		t.Errorf("message = %q", e.Message)
	}

	expectedPath := []interface{}{"pipelines", 0, "stages", 1, "options", "key"}
	if len(e.Path) != len(expectedPath) {
		t.Fatalf("path length = %d, want %d", len(e.Path), len(expectedPath))
	}
	for i, v := range expectedPath {
		if e.Path[i] != v {
			t.Errorf("path[%d] = %v, want %v", i, e.Path[i], v)
		}
	}
}

func TestValidationErrorID(t *testing.T) {
	e := ValidationError{
		Message: "err",
		Path:    []interface{}{"pipelines", 0, "stages", 1},
	}
	got := e.ID()
	want := ".pipelines[0].stages[1]"
	if got != want {
		t.Errorf("ID() = %q, want %q", got, want)
	}
}

func TestValidationErrorIDEmpty(t *testing.T) {
	e := ValidationError{Message: "err", Path: nil}
	if got := e.ID(); got != "" {
		t.Errorf("ID() = %q, want empty", got)
	}
}

func TestValidateWithECMARegex(t *testing.T) {
	s := Schema{
		Data: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"hash": map[string]interface{}{
					"type":    "string",
					"pattern": "^[a-f0-9]{64}$",
				},
			},
		},
		Name: "regex-test",
	}

	valid := map[string]interface{}{
		"hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	r := s.Validate(valid)
	if !r.Valid() {
		t.Errorf("valid hash rejected: %v", r.Errors)
	}

	invalid := map[string]interface{}{
		"hash": "not-a-hash",
	}
	r = s.Validate(invalid)
	if r.Valid() {
		t.Error("invalid hash should be rejected")
	}
}
