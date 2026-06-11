package manifest

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/supakeen/image-assembler/registry"
)

func TestParseAndDescribeGolden(t *testing.T) {
	libdir := "/usr/lib/osbuild"
	if _, err := os.Stat(libdir); os.IsNotExist(err) {
		t.Skip("osbuild libdir not found, skipping golden test")
	}

	manifestData, err := os.ReadFile("../testdata/manifest.json")
	if err != nil {
		t.Fatalf("reading test manifest: %v", err)
	}

	goldenData, err := os.ReadFile("../testdata/manifest.golden.json")
	if err != nil {
		t.Fatalf("reading golden file: %v", err)
	}

	var desc map[string]interface{}
	dec := json.NewDecoder(bytes.NewReader(manifestData))
	dec.UseNumber()
	if err := dec.Decode(&desc); err != nil {
		t.Fatalf("parsing manifest: %v", err)
	}

	reg := registry.New(libdir)
	ctx := context.Background()

	m, err := Parse(ctx, desc, reg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	result := Describe(m, true)

	goIDs := extractIDs(result)

	var golden map[string]interface{}
	if err := json.Unmarshal(goldenData, &golden); err != nil {
		t.Fatalf("parsing golden: %v", err)
	}
	pyIDs := extractIDs(golden)

	if len(goIDs) != len(pyIDs) {
		t.Fatalf("ID count mismatch: Go=%d, Python=%d", len(goIDs), len(pyIDs))
	}

	goSet := make(map[string]struct{})
	for _, id := range goIDs {
		goSet[id] = struct{}{}
	}
	pySet := make(map[string]struct{})
	for _, id := range pyIDs {
		pySet[id] = struct{}{}
	}

	if !reflect.DeepEqual(goSet, pySet) {
		t.Errorf("ID set mismatch:\n  Go:     %v\n  Python: %v", goIDs, pyIDs)
	}
	t.Logf("Verified %d stage IDs match", len(goIDs))
}

func TestParseRejectsV1(t *testing.T) {
	desc := map[string]interface{}{"version": "1"}
	reg := registry.New("/usr/lib/osbuild")
	_, err := Parse(context.Background(), desc, reg)
	if err == nil {
		t.Fatal("expected error for v1 manifest")
	}
}

func TestDepsolve(t *testing.T) {
	m := &Manifest{
		byName: make(map[string]*Pipeline),
	}

	buildPL := Pipeline{Name: "build"}
	buildPL.AddStage("org.osbuild.test", "", nil, map[string]interface{}{"key": "val"}, nil)

	imagePL := Pipeline{Name: "image"}
	buildID := buildPL.ID()
	imagePL.Build = &buildID
	imagePL.AddStage("org.osbuild.test2", "", nil, map[string]interface{}{"key2": "val2"}, nil)

	m.Pipelines = []Pipeline{buildPL, imagePL}
	m.byName["build"] = &m.Pipelines[0]
	m.byName["image"] = &m.Pipelines[1]

	emptyStore := &emptyChecker{}
	order, err := m.Depsolve(context.Background(), emptyStore, []string{"image"})
	if err != nil {
		t.Fatal(err)
	}

	if len(order) != 2 {
		t.Fatalf("expected 2 pipelines, got %d: %v", len(order), order)
	}
	if order[0] != "build" || order[1] != "image" {
		t.Errorf("expected [build image], got %v", order)
	}
}

type emptyChecker struct{}

func (e *emptyChecker) Contains(id string) bool { return false }

func extractIDs(obj interface{}) []string {
	var ids []string
	extractIDsRecursive(obj, &ids)
	return ids
}

func extractIDsRecursive(obj interface{}, ids *[]string) {
	switch v := obj.(type) {
	case map[string]interface{}:
		if id, ok := v["id"].(string); ok {
			*ids = append(*ids, id)
		}
		for _, val := range v {
			extractIDsRecursive(val, ids)
		}
	case []interface{}:
		for _, val := range v {
			extractIDsRecursive(val, ids)
		}
	}
}

