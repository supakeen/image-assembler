package monitor

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/supakeen/image-assembler/manifest"
)

const maxOutputLen = 31000

// JSONSeqMonitor emits RFC 7464 JSON text sequences (0x1E-delimited JSON
// records) for machine consumption. Each record carries a context hash,
// nested progress counters, and timing data, matching Python's JSONSeqMonitor
// output so downstream tooling (cockpit, weldr) can consume either.
type JSONSeqMonitor struct {
	out       *TextWriter
	ctx       *Context
	progress  *Progress
	mu        sync.Mutex
	startTime time.Time
	hasStart  bool
	options   map[string]interface{}
}

func NewJSONSeqMonitor(fd int, totalSteps int) *JSONSeqMonitor {
	return &JSONSeqMonitor{
		out:      NewTextWriter(fd),
		ctx:      NewContext("org.osbuild"),
		progress: &Progress{Name: "pipelines/sources", Total: totalSteps},
	}
}

func (j *JSONSeqMonitor) Begin(module Module) {
	j.ctx.SetPipeline(module.ModuleName(), module.ID())

	if p, ok := module.(*manifest.Pipeline); ok && len(p.Stages) > 0 {
		j.progress.SubProgress = &Progress{
			Name:  fmt.Sprintf("pipeline: %s", p.Name),
			Total: len(p.Stages),
		}
	}

	j.Log(fmt.Sprintf("Starting module %s", module.ModuleName()), "osbuild.monitor")
}

func (j *JSONSeqMonitor) Finish(results map[string]interface{}) {
	j.progress.Incr()

	name, _ := results["name"].(string)
	j.Log(fmt.Sprintf("Finished pipeline %s", name), "osbuild.monitor")
}

func (j *JSONSeqMonitor) Stage(stage *manifest.Stage) {
	j.ctx.SetStage(stage.Name(), stage.ID())

	if stage.Options != nil {
		j.options = make(map[string]interface{})
		for k, v := range stage.Options {
			j.options[k] = v
		}
	} else {
		j.options = nil
	}

	j.startTime = time.Now()
	j.hasStart = true

	j.Log(fmt.Sprintf("Starting module %s", stage.Name()), "osbuild.monitor")
}

func (j *JSONSeqMonitor) Result(result Result, metadata map[string]interface{}) {
	if j.progress.SubProgress != nil {
		j.progress.SubProgress.Done++
	}

	output := result.ResultOutput()
	if len(output) > maxOutputLen {
		hidden := len(output) - maxOutputLen
		output = fmt.Sprintf("[...%d bytes hidden...]\n%s", hidden, output[len(output)-maxOutputLen:])
	}

	rd := result.AsDict()
	rd["output"] = output

	var duration float64
	if j.hasStart {
		duration = time.Since(j.startTime).Seconds()
		j.hasStart = false
	}

	ctx := j.ctx.WithOrigin("osbuild.monitor")
	entry := LogEntryWithResult("", ctx, j.progress, j.options, duration, rd, metadata)

	j.jsonseq(entry)
}

func (j *JSONSeqMonitor) Log(message string, origin string) {
	ctx := j.ctx
	if origin != "" {
		ctx = ctx.WithOrigin(origin)
	}
	entry := LogEntry(message, ctx, j.progress)
	j.jsonseq(entry)
}

func (j *JSONSeqMonitor) jsonseq(entry map[string]interface{}) {
	j.mu.Lock()
	defer j.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	j.out.Write("\x1e")
	j.out.Write(string(data))
	j.out.Write("\n")
}
