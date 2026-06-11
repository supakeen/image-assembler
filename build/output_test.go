package build

import (
	"testing"

	"github.com/supakeen/image-assembler/manifest"
)

func TestFormatResultSuccess(t *testing.T) {
	m := &manifest.Manifest{
		Pipelines: []manifest.Pipeline{
			{
				Name: "build",
				Stages: []manifest.Stage{
					{Type: "org.osbuild.rpm", Options: map[string]interface{}{}},
				},
			},
		},
	}

	stage := &m.Pipelines[0].Stages[0]
	results := map[string]interface{}{
		"success": true,
		m.Pipelines[0].ID(): map[string]interface{}{
			"success": true,
			"name":    "build",
			"stages": []interface{}{
				&BuildResult{
					Stage:      stage,
					ReturnCode: 0,
					Output:     "ok\n",
				},
			},
		},
	}

	out := FormatResult(nil, m, results, nil)
	if out["type"] != "result" {
		t.Errorf("type = %v, want result", out["type"])
	}
	if out["success"] != true {
		t.Errorf("success = %v, want true", out["success"])
	}

	log, ok := out["log"].(map[string]interface{})
	if !ok {
		t.Fatal("missing log")
	}
	buildLog, ok := log["build"].([]interface{})
	if !ok {
		t.Fatal("missing build log")
	}
	if len(buildLog) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(buildLog))
	}
}

func TestFormatResultFailure(t *testing.T) {
	m := &manifest.Manifest{
		Pipelines: []manifest.Pipeline{
			{
				Name: "build",
				Stages: []manifest.Stage{
					{Type: "org.osbuild.rpm", Options: map[string]interface{}{}},
				},
			},
		},
	}

	stage := &m.Pipelines[0].Stages[0]
	results := map[string]interface{}{
		"success": false,
		m.Pipelines[0].ID(): map[string]interface{}{
			"success": false,
			"name":    "build",
			"stages": []interface{}{
				&BuildResult{
					Stage:      stage,
					ReturnCode: 1,
					Output:     "error\n",
					Error: map[string]interface{}{
						"reason": "package not found",
					},
				},
			},
		},
	}

	out := FormatResult(nil, m, results, nil)
	if out["type"] != "error" {
		t.Errorf("type = %v, want error", out["type"])
	}
	if out["success"] != false {
		t.Errorf("success = %v, want false", out["success"])
	}

	errObj, ok := out["error"].(map[string]interface{})
	if !ok {
		t.Fatal("missing error object")
	}
	if errObj["type"] != "org.osbuild.error.stage" {
		t.Errorf("error type = %v", errObj["type"])
	}
}
