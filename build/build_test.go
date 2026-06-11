package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/monitor"
	"github.com/supakeen/image-assembler/store"
)

func setupBuildStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func commitObject(t *testing.T, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	obj, err := st.New(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(obj.Tree(), "test.txt")
	if err := os.WriteFile(testFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := obj.Finalize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.Commit(ctx, obj, id); err != nil {
		t.Fatal(err)
	}
}

func TestBuildPipelineEmptyStages(t *testing.T) {
	st := setupBuildStore(t)
	p := &manifest.Pipeline{Name: "empty"}
	mon := &monitor.NullMonitor{}

	res, err := BuildPipeline(context.Background(), p, st, mon, "/usr/lib/osbuild", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["success"] != true {
		t.Errorf("success = %v, want true", res["success"])
	}
	if res["name"] != "empty" {
		t.Errorf("name = %v, want empty", res["name"])
	}
}

func TestBuildPipelineCacheHit(t *testing.T) {
	st := setupBuildStore(t)

	p := &manifest.Pipeline{Name: "build"}
	p.Stages = []manifest.Stage{
		{
			Type:    "org.osbuild.rpm",
			Options: map[string]interface{}{"key": "val"},
			Inputs:  manifest.NewOrderedMap[*manifest.Input](),
			Devices: manifest.NewOrderedMap[*manifest.Device](),
			Mounts:  manifest.NewOrderedMap[*manifest.Mount](),
		},
	}
	pipelineID := p.ID()

	commitObject(t, st, pipelineID)

	if !st.Contains(pipelineID) {
		t.Fatal("object should be in store after commit")
	}

	mon := &monitor.NullMonitor{}
	res, err := BuildPipeline(context.Background(), p, st, mon, "/usr/lib/osbuild", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["success"] != true {
		t.Errorf("success = %v, want true", res["success"])
	}
	if _, hasStages := res["stages"]; hasStages {
		t.Error("cached pipeline should not have stages in result")
	}
}

func TestBuildCachedPipeline(t *testing.T) {
	st := setupBuildStore(t)

	p := manifest.Pipeline{Name: "build"}
	p.Stages = []manifest.Stage{
		{
			Type:    "org.osbuild.test",
			Options: map[string]interface{}{"k": "v"},
			Inputs:  manifest.NewOrderedMap[*manifest.Input](),
			Devices: manifest.NewOrderedMap[*manifest.Device](),
			Mounts:  manifest.NewOrderedMap[*manifest.Mount](),
		},
	}
	pipelineID := p.ID()
	commitObject(t, st, pipelineID)

	m := &manifest.Manifest{
		Pipelines: []manifest.Pipeline{p},
	}

	mon := &monitor.NullMonitor{}
	res, err := Build(context.Background(), m, st, []string{pipelineID}, mon, "/usr/lib/osbuild", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["success"] != true {
		t.Errorf("success = %v, want true", res["success"])
	}
}

func TestBuildPipelineNotFound(t *testing.T) {
	st := setupBuildStore(t)
	m := &manifest.Manifest{}

	mon := &monitor.NullMonitor{}
	_, err := Build(context.Background(), m, st, []string{"nonexistent"}, mon, "/usr/lib/osbuild", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent pipeline")
	}
}
