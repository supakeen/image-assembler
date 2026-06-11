package monitor

import (
	"os"
	"strings"
	"testing"

	"github.com/supakeen/image-assembler/manifest"
)

func TestLogMonitorBegin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	lm := &LogMonitor{out: &TextWriter{file: w, IsATTY: false}}

	p := &manifest.Pipeline{
		Name: "build",
		Stages: []manifest.Stage{
			{Type: "org.osbuild.rpm", Options: map[string]interface{}{}},
		},
	}

	lm.Begin(p)
	w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "Pipeline build:") {
		t.Errorf("missing pipeline header in: %q", output)
	}
	if !strings.Contains(output, "root: <host>") {
		t.Errorf("missing host root in: %q", output)
	}
}

func TestLogMonitorStageAndResult(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	lm := &LogMonitor{out: &TextWriter{file: w, IsATTY: false}}

	s := &manifest.Stage{
		Type:    "org.osbuild.rpm",
		Options: map[string]interface{}{},
	}

	lm.Stage(s)
	lm.Result(nil, nil)
	w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "org.osbuild.rpm:") {
		t.Errorf("missing stage name in: %q", output)
	}
	if !strings.Contains(output, "Duration:") {
		t.Errorf("missing duration in: %q", output)
	}
}
