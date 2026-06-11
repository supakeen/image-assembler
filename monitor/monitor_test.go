package monitor

import (
	"os"
	"testing"

	"github.com/supakeen/image-assembler/manifest"
)

func TestNullMonitorNoPanic(t *testing.T) {
	n := &NullMonitor{}
	p := &manifest.Pipeline{Name: "test"}
	s := &manifest.Stage{Type: "org.osbuild.rpm"}

	n.Begin(p)
	n.Stage(s)
	n.Result(nil, nil)
	n.Finish(nil)
	n.Log("hello", "test")
}

func TestMakeFactory(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"NullMonitor", false},
		{"LogMonitor", false},
		{"JSONSeqMonitor", false},
		{"Unknown", true},
	}

	for _, tt := range tests {
		m, err := Make(tt.name, int(os.Stdout.Fd()), 1)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Make(%q) should error", tt.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("Make(%q) error: %v", tt.name, err)
			continue
		}
		if m == nil {
			t.Errorf("Make(%q) returned nil", tt.name)
		}
	}
}

func TestTextWriterWrite(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	tw := &TextWriter{file: w, IsATTY: false}
	tw.Write("hello world")

	buf := make([]byte, 100)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "hello world" {
		t.Errorf("got %q, want %q", string(buf[:n]), "hello world")
	}
}

func TestTextWriterTermNoTTY(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	tw := &TextWriter{file: w, IsATTY: false}
	tw.Term("should not appear")

	w.Close()
	buf := make([]byte, 100)
	n, _ := r.Read(buf)
	if n != 0 {
		t.Errorf("Term wrote %d bytes to non-TTY", n)
	}
}
