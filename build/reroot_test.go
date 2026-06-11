package build

import (
	"testing"
)

func TestRerootJoinsPaths(t *testing.T) {
	args := map[string]interface{}{
		"paths": map[string]interface{}{
			"devices": "/dev",
			"inputs":  "/run/osbuild/inputs",
		},
		"devices": map[string]interface{}{
			"disk": map[string]interface{}{"path": "loop0"},
		},
		"inputs": map[string]interface{}{
			"rpm": map[string]interface{}{"path": "packages"},
		},
	}

	reroot(args)

	devices := args["devices"].(map[string]interface{})
	disk := devices["disk"].(map[string]interface{})
	if disk["path"] != "/dev/loop0" {
		t.Errorf("device path = %v, want /dev/loop0", disk["path"])
	}

	inputs := args["inputs"].(map[string]interface{})
	rpm := inputs["rpm"].(map[string]interface{})
	if rpm["path"] != "/run/osbuild/inputs/packages" {
		t.Errorf("input path = %v, want /run/osbuild/inputs/packages", rpm["path"])
	}
}

func TestRerootMissingPaths(t *testing.T) {
	args := map[string]interface{}{
		"devices": map[string]interface{}{
			"disk": map[string]interface{}{"path": "loop0"},
		},
	}

	reroot(args)

	devices := args["devices"].(map[string]interface{})
	disk := devices["disk"].(map[string]interface{})
	if disk["path"] != "loop0" {
		t.Errorf("path should be unchanged without paths key, got %v", disk["path"])
	}
}

func TestRerootEmptyPath(t *testing.T) {
	args := map[string]interface{}{
		"paths": map[string]interface{}{
			"devices": "/dev",
		},
		"devices": map[string]interface{}{
			"disk": map[string]interface{}{"path": ""},
		},
	}

	reroot(args)

	devices := args["devices"].(map[string]interface{})
	disk := devices["disk"].(map[string]interface{})
	if disk["path"] != "" {
		t.Errorf("empty path should stay empty, got %v", disk["path"])
	}
}

func TestRerootNonMapItem(t *testing.T) {
	args := map[string]interface{}{
		"paths": map[string]interface{}{
			"devices": "/dev",
		},
		"devices": map[string]interface{}{
			"disk": "not a map",
		},
	}

	reroot(args)
}

func TestRerootNoPathField(t *testing.T) {
	args := map[string]interface{}{
		"paths": map[string]interface{}{
			"devices": "/dev",
		},
		"devices": map[string]interface{}{
			"disk": map[string]interface{}{"name": "loop0"},
		},
	}

	reroot(args)

	devices := args["devices"].(map[string]interface{})
	disk := devices["disk"].(map[string]interface{})
	if _, ok := disk["path"]; ok {
		t.Error("should not create path field when none existed")
	}
}
