// Package monitor provides user-facing build progress output, separate from
// diagnostic logging. The Monitor interface defines a lifecycle that the
// build system drives: Begin/Stage/Result/Finish per pipeline, with Log for
// free-form messages. Multiple implementations format output differently
// (human-readable text vs JSON-seq for machine consumption).
package monitor

import (
	"fmt"
	"os"

	"github.com/supakeen/image-assembler/manifest"
)

// Result abstracts stage and source outcomes so monitors can report
// progress without depending on concrete build types.
type Result interface {
	ResultName() string
	ResultID() string
	Success() bool
	ResultOutput() string
	AsDict() map[string]interface{}
}

// Module abstracts both Pipeline and Source for the Begin call.
// Pipeline has a Name field (so can't use Name() as the method), hence
// ModuleName() to avoid the collision.
type Module interface {
	ModuleName() string
	ID() string
}

// Monitor receives build progress events. Implementations must be safe for
// sequential use (not concurrent — the build system calls methods in order).
type Monitor interface {
	Begin(module Module)
	Finish(results map[string]interface{})
	Stage(stage *manifest.Stage)
	Result(result Result, metadata map[string]interface{})
	Log(message string, origin string)
}

// NullMonitor silently discards all progress events. Used with --json or
// --quiet flags, or when only log output is desired.
type NullMonitor struct{}

func (n *NullMonitor) Begin(_ Module)                                        {}
func (n *NullMonitor) Finish(results map[string]interface{})                  {}
func (n *NullMonitor) Stage(stage *manifest.Stage)                            {}
func (n *NullMonitor) Result(result Result, metadata map[string]interface{})  {}
func (n *NullMonitor) Log(message string, origin string)                      {}

// TextWriter wraps a file descriptor for monitor output, with TTY detection
// so terminal-only formatting (like cursor movement) can be skipped when
// output is piped.
type TextWriter struct {
	file   *os.File
	IsATTY bool
}

func NewTextWriter(fd int) *TextWriter {
	f := os.NewFile(uintptr(fd), fmt.Sprintf("fd/%d", fd))
	fi, err := f.Stat()
	isatty := err == nil && (fi.Mode()&os.ModeCharDevice) != 0
	return &TextWriter{
		file:   f,
		IsATTY: isatty,
	}
}

func (w *TextWriter) Write(text string) {
	data := []byte(text)
	for len(data) > 0 {
		n, err := w.file.Write(data)
		if err != nil {
			return
		}
		data = data[n:]
	}
}

func (w *TextWriter) Term(text string) {
	if w.IsATTY {
		w.Write(text)
	}
}


// Make creates a Monitor by name, matching Python's --monitor flag values.
func Make(name string, fd int, totalSteps int) (Monitor, error) {
	switch name {
	case "NullMonitor":
		return &NullMonitor{}, nil
	case "LogMonitor":
		return NewLogMonitor(fd, totalSteps), nil
	case "JSONSeqMonitor":
		return NewJSONSeqMonitor(fd, totalSteps), nil
	default:
		return nil, fmt.Errorf("unknown monitor: %q", name)
	}
}
