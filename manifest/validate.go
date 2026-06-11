package manifest

import (
	"context"
	"fmt"

	"github.com/supakeen/image-assembler/registry"
)

// Validate checks a raw manifest against the JSON Schema for every module
// it references (stages, sources, devices, inputs, mounts). Validation
// happens before parsing so schema errors are reported with their original
// JSON paths rather than post-parse structure.
func Validate(ctx context.Context, desc map[string]interface{}, reg *registry.Registry) *registry.ValidationResult {
	result := &registry.ValidationResult{Origin: "osbuild"}

	manifestSchema := reg.Schema("Manifest", "", "2")
	if manifestSchema.Data != nil {
		r := manifestSchema.Validate(desc)
		result.Merge(r)
	}

	if sources, ok := desc["sources"].(map[string]interface{}); ok {
		for name, sdesc := range sources {
			if err := ctx.Err(); err != nil {
				result.Errors = append(result.Errors, registry.ValidationError{
					Message: err.Error(),
				})
				return result
			}
			s := reg.Schema("Source", name, "2")
			if s.Data != nil {
				r := s.Validate(sdesc)
				result.Merge(r, "sources", name)
			}
		}
	}

	if pipelines, ok := desc["pipelines"].([]interface{}); ok {
		for pi, pdesc := range pipelines {
			pd, ok := pdesc.(map[string]interface{})
			if !ok {
				continue
			}

			stages, ok := pd["stages"].([]interface{})
			if !ok {
				continue
			}

			for si, sdesc := range stages {
				if err := ctx.Err(); err != nil {
					result.Errors = append(result.Errors, registry.ValidationError{
						Message: err.Error(),
					})
					return result
				}

				sd, ok := sdesc.(map[string]interface{})
				if !ok {
					continue
				}

				stageType, _ := sd["type"].(string)
				s := reg.Schema("Stage", stageType, "2")
				if s.Data != nil {
					r := s.Validate(sd)
					result.Merge(r, "pipelines", pi, "stages", si)
				}

				validateStageModules(result, sd, reg, "Device", "devices", pi, si)
				validateStageModules(result, sd, reg, "Input", "inputs", pi, si)
				validateStageMounts(result, sd, reg, pi, si)
			}
		}
	}

	return result
}

func validateStageModules(result *registry.ValidationResult, stage map[string]interface{}, reg *registry.Registry, kind, field string, pi, si int) {
	items, ok := stage[field].(map[string]interface{})
	if !ok {
		return
	}

	for name, idesc := range items {
		id, ok := idesc.(map[string]interface{})
		if !ok {
			continue
		}
		itemType, _ := id["type"].(string)
		if itemType == "" {
			continue
		}
		s := reg.Schema(kind, itemType, "2")
		if s.Data != nil {
			r := s.Validate(id)
			result.Merge(r, "pipelines", pi, "stages", si, field, name)
		}
	}
}

func validateStageMounts(result *registry.ValidationResult, stage map[string]interface{}, reg *registry.Registry, pi, si int) {
	mounts, ok := stage["mounts"].([]interface{})
	if !ok {
		return
	}

	for mi, mdesc := range mounts {
		md, ok := mdesc.(map[string]interface{})
		if !ok {
			continue
		}
		mountType, _ := md["type"].(string)
		if mountType == "" {
			continue
		}
		s := reg.Schema("Mount", mountType, "2")
		if s.Data != nil {
			r := s.Validate(md)
			result.Merge(r, "pipelines", pi, "stages", si, "mounts", mi)
		}
	}
}

func FormatValidationErrors(result *registry.ValidationResult) string {
	if result.Valid() {
		return ""
	}
	msg := "Validation failed:\n"
	for _, e := range result.Errors {
		id := e.ID()
		if id == "" {
			msg += fmt.Sprintf("  %s\n", e.Message)
		} else {
			msg += fmt.Sprintf("  %s: %s\n", id, e.Message)
		}
	}
	return msg
}
