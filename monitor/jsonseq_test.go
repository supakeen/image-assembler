package monitor

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/supakeen/image-assembler/manifest"
)

type mockResult struct {
	name    string
	id      string
	success bool
	output  string
}

func (r *mockResult) ResultName() string   { return r.name }
func (r *mockResult) ResultID() string     { return r.id }
func (r *mockResult) Success() bool        { return r.success }
func (r *mockResult) ResultOutput() string { return r.output }
func (r *mockResult) AsDict() map[string]interface{} {
	return map[string]interface{}{
		"name":    r.name,
		"id":      r.id,
		"success": r.success,
		"output":  r.output,
	}
}

func TestJSONSeqMonitorFormat(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	jsm := &JSONSeqMonitor{
		out:      &TextWriter{file: w, IsATTY: false},
		ctx:      NewContext("org.osbuild"),
		progress: &Progress{Name: "pipelines/sources", Total: 2},
	}

	p := &manifest.Pipeline{
		Name: "build",
		Stages: []manifest.Stage{
			{Type: "org.osbuild.rpm", Options: map[string]interface{}{}},
		},
	}

	jsm.Begin(p)
	w.Close()

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	records := splitRecords(output)
	if len(records) == 0 {
		t.Fatal("no JSON-seq records")
	}

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(records[0]), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := entry["message"]; !ok {
		t.Error("missing message")
	}
	if _, ok := entry["context"]; !ok {
		t.Error("missing context")
	}
	if _, ok := entry["progress"]; !ok {
		t.Error("missing progress")
	}
	if _, ok := entry["timestamp"]; !ok {
		t.Error("missing timestamp")
	}
}

func TestJSONSeqMonitorContextDedup(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	jsm := &JSONSeqMonitor{
		out:      &TextWriter{file: w, IsATTY: false},
		ctx:      NewContext("org.osbuild"),
		progress: &Progress{Name: "test", Total: 1},
	}

	jsm.Log("first", "org.osbuild")
	jsm.Log("second", "org.osbuild")
	w.Close()

	buf := make([]byte, 16384)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	records := splitRecords(output)
	if len(records) < 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	var first, second map[string]interface{}
	json.Unmarshal([]byte(records[0]), &first)
	json.Unmarshal([]byte(records[1]), &second)

	firstCtx, _ := first["context"].(map[string]interface{})
	secondCtx, _ := second["context"].(map[string]interface{})

	if _, ok := firstCtx["origin"]; !ok {
		t.Error("first context should have origin")
	}
	if _, ok := secondCtx["origin"]; ok {
		t.Error("second context should be id-only (no origin)")
	}
	if _, ok := secondCtx["id"]; !ok {
		t.Error("second context should have id")
	}
}

func TestJSONSeqMonitorOutputTruncation(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	jsm := &JSONSeqMonitor{
		out:      &TextWriter{file: w, IsATTY: false},
		ctx:      NewContext("org.osbuild"),
		progress: &Progress{Name: "test", Total: 1},
	}

	s := &manifest.Stage{Type: "org.osbuild.rpm", Options: map[string]interface{}{}}
	jsm.Stage(s)

	longOutput := strings.Repeat("x", 50000)
	result := &mockResult{
		name:    "org.osbuild.rpm",
		id:      "abc123",
		success: true,
		output:  longOutput,
	}

	jsm.Result(result, nil)
	w.Close()

	buf := make([]byte, 65536)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	records := splitRecords(output)
	if len(records) < 2 {
		t.Fatalf("expected at least 2 records, got %d", len(records))
	}

	var entry map[string]interface{}
	json.Unmarshal([]byte(records[len(records)-1]), &entry)

	res, _ := entry["result"].(map[string]interface{})
	if res == nil {
		t.Fatal("missing result in entry")
	}
	resultOutput, _ := res["output"].(string)
	if !strings.Contains(resultOutput, "bytes hidden") {
		t.Error("long output should be truncated with 'bytes hidden' marker")
	}
}

func splitRecords(data string) []string {
	parts := strings.Split(data, "\x1e")
	var records []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			records = append(records, p)
		}
	}
	return records
}
