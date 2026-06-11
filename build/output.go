package build

import (
	"context"

	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/store"
)

// FormatResult transforms the internal build results into the JSON output
// format expected by --json consumers, matching Python osbuild's output
// structure with "type", "success", optional "metadata", "log", and "error".
func FormatResult(ctx context.Context, m *manifest.Manifest,
	results map[string]interface{}, st *store.Store) map[string]interface{} {

	success, _ := results["success"].(bool)

	if !success {
		return formatFailure(results)
	}
	return formatSuccess(ctx, m, results, st)
}

func formatFailure(results map[string]interface{}) map[string]interface{} {
	var lastRes map[string]interface{}
	for k, v := range results {
		if k == "success" {
			continue
		}
		if r, ok := v.(map[string]interface{}); ok {
			lastRes = r
		}
	}

	if lastRes == nil {
		return map[string]interface{}{
			"type":    "error",
			"success": false,
		}
	}

	stages, _ := lastRes["stages"].([]interface{})
	if len(stages) == 0 {
		return map[string]interface{}{
			"type":    "error",
			"success": false,
		}
	}

	last := stages[len(stages)-1]
	var failed map[string]interface{}

	switch f := last.(type) {
	case *BuildResult:
		failed = map[string]interface{}{
			"id":     f.Stage.ID(),
			"type":   f.Stage.Name(),
			"output": f.Output,
			"error":  f.Error,
		}
	case map[string]interface{}:
		failed = f
	}

	return map[string]interface{}{
		"type":    "error",
		"success": false,
		"error": map[string]interface{}{
			"type": "org.osbuild.error.stage",
			"details": map[string]interface{}{
				"stage": failed,
			},
		},
	}
}

func formatSuccess(ctx context.Context, m *manifest.Manifest,
	results map[string]interface{}, st *store.Store) map[string]interface{} {

	metadata := make(map[string]interface{})
	log := make(map[string]interface{})

	for i := range m.Pipelines {
		p := &m.Pipelines[i]

		if st != nil {
			obj, err := st.Get(ctx, p.ID())
			if err == nil && obj != nil {
				data := make(map[string]interface{})
				for j := range p.Stages {
					md, _ := obj.Meta().Get(p.Stages[j].ID())
					if md != nil {
						data[p.Stages[j].Name()] = md
					}
				}
				if len(data) > 0 {
					metadata[p.Name] = data
				}
			}
		}

		pipeRes, _ := results[p.ID()].(map[string]interface{})
		if pipeRes == nil {
			continue
		}

		stages, _ := pipeRes["stages"].([]interface{})
		var entries []interface{}

		for _, s := range stages {
			switch sr := s.(type) {
			case *BuildResult:
				entry := map[string]interface{}{
					"id":     sr.Stage.ID(),
					"type":   sr.Stage.Name(),
					"output": sr.Output,
				}
				if !sr.Success() {
					entry["success"] = false
					if sr.Error != nil {
						entry["error"] = sr.Error
					}
				}
				entries = append(entries, entry)
			}
		}

		if len(entries) > 0 {
			log[p.Name] = entries
		}
	}

	result := map[string]interface{}{
		"type":    "result",
		"success": true,
	}
	if len(metadata) > 0 {
		result["metadata"] = metadata
	}
	if len(log) > 0 {
		result["log"] = log
	}

	return result
}
