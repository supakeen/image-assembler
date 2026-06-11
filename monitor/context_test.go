package monitor

import "testing"

func TestContextIDStable(t *testing.T) {
	c := NewContext("org.osbuild")
	c.SetPipeline("build", "abc123")
	c.SetStage("org.osbuild.rpm", "def456")

	id1 := c.ID()
	id2 := c.ID()
	if id1 != id2 {
		t.Errorf("ID not stable: %q != %q", id1, id2)
	}
	if len(id1) != 64 {
		t.Errorf("ID not SHA256 hex: len=%d", len(id1))
	}
}

func TestContextIDChanges(t *testing.T) {
	c := NewContext("org.osbuild")
	c.SetPipeline("build", "abc123")
	id1 := c.ID()

	c.SetPipeline("os", "xyz789")
	id2 := c.ID()

	if id1 == id2 {
		t.Error("ID should change when pipeline changes")
	}
}

func TestContextAsDictFirstFull(t *testing.T) {
	c := NewContext("org.osbuild")
	c.SetPipeline("build", "abc123")

	d := c.AsDict()
	if _, ok := d["origin"]; !ok {
		t.Error("first AsDict should include origin")
	}
	if _, ok := d["pipeline"]; !ok {
		t.Error("first AsDict should include pipeline")
	}
}

func TestContextAsDictSecondIDOnly(t *testing.T) {
	c := NewContext("org.osbuild")
	c.SetPipeline("build", "abc123")

	c.AsDict()
	d := c.AsDict()

	if _, ok := d["origin"]; ok {
		t.Error("second AsDict should be id-only, but has origin")
	}
	if _, ok := d["id"]; !ok {
		t.Error("second AsDict should have id")
	}
}

func TestContextWithOrigin(t *testing.T) {
	c := NewContext("org.osbuild")
	c.SetPipeline("build", "abc123")

	same := c.WithOrigin("org.osbuild")
	if same != c {
		t.Error("WithOrigin with same origin should return same context")
	}

	diff := c.WithOrigin("osbuild.monitor")
	if diff == c {
		t.Error("WithOrigin with different origin should return new context")
	}
	if diff.Origin != "osbuild.monitor" {
		t.Errorf("new context origin = %q, want osbuild.monitor", diff.Origin)
	}
	if diff.PipelineName != c.PipelineName {
		t.Error("new context should preserve pipeline name")
	}
}

func TestContextWithOriginEmpty(t *testing.T) {
	c := NewContext("org.osbuild")
	same := c.WithOrigin("")
	if same != c {
		t.Error("WithOrigin with empty origin should return same context")
	}
}

func TestProgressIncr(t *testing.T) {
	p := &Progress{Name: "test", Total: 5, Done: 0}
	p.SubProgress = &Progress{Name: "sub", Total: 3}

	p.Incr()
	if p.Done != 1 {
		t.Errorf("Done = %d, want 1", p.Done)
	}
	if p.SubProgress != nil {
		t.Error("SubProgress should be nil after Incr")
	}
}

func TestProgressAsDict(t *testing.T) {
	p := &Progress{Name: "pipelines/sources", Total: 5, Done: 2}
	p.SubProgress = &Progress{Name: "pipeline: build", Total: 10, Done: 3}

	d := p.AsDict()
	if d["name"] != "pipelines/sources" {
		t.Errorf("name = %v", d["name"])
	}
	if d["total"] != 5 {
		t.Errorf("total = %v", d["total"])
	}
	if d["done"] != 2 {
		t.Errorf("done = %v", d["done"])
	}
	sub, ok := d["progress"].(map[string]interface{})
	if !ok {
		t.Fatal("missing sub progress")
	}
	if sub["name"] != "pipeline: build" {
		t.Errorf("sub name = %v", sub["name"])
	}
}

func TestLogEntry(t *testing.T) {
	c := NewContext("org.osbuild")
	p := &Progress{Name: "test", Total: 1}

	entry := LogEntry("hello", c, p)
	if entry["message"] != "hello" {
		t.Errorf("message = %v", entry["message"])
	}
	if _, ok := entry["timestamp"]; !ok {
		t.Error("missing timestamp")
	}
	if _, ok := entry["context"]; !ok {
		t.Error("missing context")
	}
	if _, ok := entry["progress"]; !ok {
		t.Error("missing progress")
	}
}
