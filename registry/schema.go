package registry

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dlclark/regexp2"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type ecmaRegexp regexp2.Regexp

func (re *ecmaRegexp) MatchString(s string) bool {
	matched, _ := (*regexp2.Regexp)(re).MatchString(s)
	return matched
}

func (re *ecmaRegexp) String() string {
	return (*regexp2.Regexp)(re).String()
}

func ecmaCompile(s string) (jsonschema.Regexp, error) {
	re, err := regexp2.Compile(s, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	return (*ecmaRegexp)(re), nil
}

// Schema wraps a JSON Schema document for validating manifest fragments.
// It uses Draft 4 with ECMAScript regex semantics to match the Python
// jsonschema library's behavior.
type Schema struct {
	Data map[string]interface{}
	Name string
}

// ValidationResult collects errors from schema validation. A nil Errors
// slice indicates a valid document.
type ValidationResult struct {
	Origin string
	Errors []ValidationError
}

func (r *ValidationResult) Valid() bool {
	return len(r.Errors) == 0
}

func (r *ValidationResult) Merge(other *ValidationResult, pathPrefix ...interface{}) {
	for _, e := range other.Errors {
		ne := ValidationError{
			Message: e.Message,
			Path:    make([]interface{}, 0, len(pathPrefix)+len(e.Path)),
		}
		ne.Path = append(ne.Path, pathPrefix...)
		ne.Path = append(ne.Path, e.Path...)
		r.Errors = append(r.Errors, ne)
	}
}

type ValidationError struct {
	Message string
	Path    []interface{}
}

func (e *ValidationError) ID() string {
	var b strings.Builder
	for _, p := range e.Path {
		switch v := p.(type) {
		case string:
			b.WriteString(".")
			b.WriteString(v)
		case int:
			fmt.Fprintf(&b, "[%d]", v)
		}
	}
	return b.String()
}

func (s *Schema) Validate(target interface{}) *ValidationResult {
	result := &ValidationResult{Origin: s.Name}

	if s.Data == nil {
		return result
	}

	schemaJSON, err := json.Marshal(s.Data)
	if err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Message: fmt.Sprintf("invalid schema: %v", err),
		})
		return result
	}

	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft4)
	c.UseRegexpEngine(ecmaCompile)
	schemaURL := "schema.json"
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(schemaJSON)))
	if err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Message: fmt.Sprintf("schema parse error: %v", err),
		})
		return result
	}
	if err := c.AddResource(schemaURL, doc); err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Message: fmt.Sprintf("schema compilation error: %v", err),
		})
		return result
	}

	sch, err := c.Compile(schemaURL)
	if err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Message: fmt.Sprintf("schema compilation error: %v", err),
		})
		return result
	}

	if err := sch.Validate(target); err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			collectErrors(result, ve)
		} else {
			result.Errors = append(result.Errors, ValidationError{
				Message: err.Error(),
			})
		}
	}

	return result
}

func collectErrors(result *ValidationResult, ve *jsonschema.ValidationError) {
	if len(ve.Causes) == 0 {
		result.Errors = append(result.Errors, ValidationError{
			Message: ve.Error(),
		})
		return
	}
	for _, cause := range ve.Causes {
		collectErrors(result, cause)
	}
}
