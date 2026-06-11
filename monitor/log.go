package monitor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/supakeen/image-assembler/manifest"
)

// LogMonitor prints human-readable build progress to a file descriptor,
// including pipeline metadata, stage options, and per-stage timing.
type LogMonitor struct {
	out       *TextWriter
	startTime time.Time
	hasStart  bool
}

func NewLogMonitor(fd int, totalSteps int) *LogMonitor {
	return &LogMonitor{
		out: NewTextWriter(fd),
	}
}

func (l *LogMonitor) Begin(module Module) {
	if p, ok := module.(*manifest.Pipeline); ok {
		l.beginPipeline(p)
		return
	}
	l.out.Write(fmt.Sprintf("%s: %s\n", module.ModuleName(), module.ID()))
}

func (l *LogMonitor) beginPipeline(pipeline *manifest.Pipeline) {
	l.out.Write(fmt.Sprintf("Pipeline %s: %s\n", pipeline.Name, pipeline.ID()))
	l.out.Write("Build\n")

	if pipeline.Build != nil {
		l.out.Write(fmt.Sprintf("  root: %s\n", *pipeline.Build))
	} else {
		l.out.Write("  root: <host>\n")
	}

	if pipeline.Runner.Name != "" {
		l.out.Write(fmt.Sprintf("  runner: %s (%s)\n", pipeline.Runner.Name, pipeline.Runner.Exec()))
	}

	if pipeline.SourceEpoch != nil {
		t := time.Unix(int64(*pipeline.SourceEpoch), 0).UTC()
		l.out.Write(fmt.Sprintf("  source-epoch: %s [%d]\n", t.Format(time.RFC3339), *pipeline.SourceEpoch))
	}
}

func (l *LogMonitor) Finish(results map[string]interface{}) {}

func (l *LogMonitor) Stage(stage *manifest.Stage) {
	opts := ""
	if len(stage.Options) > 0 {
		data, _ := json.MarshalIndent(stage.Options, "", "  ")
		opts = " " + string(data)
	}
	l.out.Write(fmt.Sprintf("%s: %s%s\n", stage.Name(), stage.ID(), opts))
	l.startTime = time.Now()
	l.hasStart = true
}

func (l *LogMonitor) Result(result Result, metadata map[string]interface{}) {
	if l.hasStart {
		duration := time.Since(l.startTime).Seconds()
		l.out.Write(fmt.Sprintf("\n⏱  Duration: %.2fs\n", duration))
		l.hasStart = false
	}
}

func (l *LogMonitor) Log(message string, origin string) {
	l.out.Write(message)
}
