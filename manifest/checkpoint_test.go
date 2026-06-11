package manifest

import "testing"

func TestMarkCheckpointsExactName(t *testing.T) {
	m := &Manifest{
		Pipelines: []Pipeline{
			{
				Name: "build",
				Stages: []Stage{
					{Type: "org.osbuild.rpm", Options: map[string]interface{}{}},
					{Type: "org.osbuild.fix-bls", Options: map[string]interface{}{}},
				},
			},
		},
	}

	marked := m.MarkCheckpoints([]string{"build"})
	if len(marked) != 1 {
		t.Fatalf("expected 1 match, got %d", len(marked))
	}

	last := &m.Pipelines[0].Stages[len(m.Pipelines[0].Stages)-1]
	if !last.Checkpoint {
		t.Error("last stage should be marked as checkpoint")
	}
	if _, ok := marked[last.ID()]; !ok {
		t.Error("marked set should contain last stage ID")
	}
}

func TestMarkCheckpointsGlob(t *testing.T) {
	m := &Manifest{
		Pipelines: []Pipeline{
			{
				Name: "build",
				Stages: []Stage{
					{Type: "org.osbuild.rpm", Options: map[string]interface{}{}},
				},
			},
			{
				Name: "os",
				Stages: []Stage{
					{Type: "org.osbuild.locale", Options: map[string]interface{}{}},
				},
			},
		},
	}

	marked := m.MarkCheckpoints([]string{"*"})
	if len(marked) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(marked))
	}
}

func TestMarkCheckpointsStageName(t *testing.T) {
	m := &Manifest{
		Pipelines: []Pipeline{
			{
				Name: "build",
				Stages: []Stage{
					{Type: "org.osbuild.rpm", Options: map[string]interface{}{}},
					{Type: "org.osbuild.fix-bls", Options: map[string]interface{}{}},
				},
			},
		},
	}

	marked := m.MarkCheckpoints([]string{"org.osbuild.rpm"})
	if len(marked) != 1 {
		t.Fatalf("expected 1 match, got %d", len(marked))
	}

	first := &m.Pipelines[0].Stages[0]
	if !first.Checkpoint {
		t.Error("first stage should be marked as checkpoint")
	}
}

func TestMarkCheckpointsNoMatch(t *testing.T) {
	m := &Manifest{
		Pipelines: []Pipeline{
			{
				Name: "build",
				Stages: []Stage{
					{Type: "org.osbuild.rpm", Options: map[string]interface{}{}},
				},
			},
		},
	}

	marked := m.MarkCheckpoints([]string{"nonexistent"})
	if len(marked) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(marked))
	}
}

func TestMarkCheckpointsEmptyPipeline(t *testing.T) {
	m := &Manifest{
		Pipelines: []Pipeline{
			{Name: "empty"},
		},
	}

	marked := m.MarkCheckpoints([]string{"empty"})
	if len(marked) != 0 {
		t.Fatalf("expected 0 matches for empty pipeline, got %d", len(marked))
	}
}
