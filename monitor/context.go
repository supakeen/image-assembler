package monitor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// Context tracks the current position in the build (pipeline + stage) and
// produces a deterministic hash ID from that position. The hash is used by
// JSONSeqMonitor to let consumers correlate log entries without parsing
// human-readable names. Repeated calls with the same position return a
// compact form (ID only) to reduce output volume.
type Context struct {
	Origin       string
	PipelineName string
	PipelineID   string
	StageName    string
	StageID      string
	idCache      string
	idHistory    map[string]struct{}
}

func NewContext(origin string) *Context {
	return &Context{
		Origin:    origin,
		idHistory: make(map[string]struct{}),
	}
}

func (c *Context) ID() string {
	if c.idCache != "" {
		return c.idCache
	}
	d := map[string]interface{}{
		"origin": c.Origin,
		"pipeline": map[string]interface{}{
			"name": c.PipelineName,
			"id":   c.PipelineID,
			"stage": map[string]interface{}{
				"name": c.StageName,
				"id":   c.StageID,
			},
		},
	}
	data, _ := json.Marshal(d)
	h := sha256.Sum256(data)
	c.idCache = fmt.Sprintf("%x", h)
	return c.idCache
}

func (c *Context) invalidate() {
	c.idCache = ""
}

func (c *Context) SetPipeline(name, id string) {
	c.PipelineName = name
	c.PipelineID = id
	c.StageName = ""
	c.StageID = ""
	c.invalidate()
}

func (c *Context) SetStage(name, id string) {
	c.StageName = name
	c.StageID = id
	c.invalidate()
}

func (c *Context) AsDict() map[string]interface{} {
	id := c.ID()

	if _, seen := c.idHistory[id]; seen {
		return map[string]interface{}{"id": id}
	}

	c.idHistory[id] = struct{}{}

	result := map[string]interface{}{
		"id":     id,
		"origin": c.Origin,
	}

	pipeline := map[string]interface{}{
		"name": c.PipelineName,
		"id":   c.PipelineID,
	}
	if c.StageName != "" || c.StageID != "" {
		pipeline["stage"] = map[string]interface{}{
			"name": c.StageName,
			"id":   c.StageID,
		}
	}
	result["pipeline"] = pipeline

	return result
}

func (c *Context) WithOrigin(origin string) *Context {
	if origin == "" || origin == c.Origin {
		return c
	}
	return &Context{
		Origin:       origin,
		PipelineName: c.PipelineName,
		PipelineID:   c.PipelineID,
		StageName:    c.StageName,
		StageID:      c.StageID,
		idHistory:    c.idHistory,
	}
}

// Progress tracks completion of a countable unit of work (pipelines, stages)
// with optional nesting for sub-progress within a pipeline.
type Progress struct {
	Name        string
	Total       int
	Done        int
	SubProgress *Progress
}

func (p *Progress) Incr() {
	p.Done++
	p.SubProgress = nil
}

func (p *Progress) AsDict() map[string]interface{} {
	d := map[string]interface{}{
		"name":  p.Name,
		"total": p.Total,
		"done":  p.Done,
	}
	if p.SubProgress != nil {
		d["progress"] = p.SubProgress.AsDict()
	}
	return d
}

func LogEntry(message string, ctx *Context, progress *Progress) map[string]interface{} {
	entry := make(map[string]interface{})
	if message != "" {
		entry["message"] = message
	}
	if ctx != nil {
		entry["context"] = ctx.AsDict()
	}
	if progress != nil {
		entry["progress"] = progress.AsDict()
	}
	now := time.Now()
	entry["timestamp"] = float64(now.Unix()) + float64(now.Nanosecond())/1e9
	return entry
}

func LogEntryWithResult(message string, ctx *Context, progress *Progress,
	options map[string]interface{}, duration float64,
	result map[string]interface{}, metadata map[string]interface{}) map[string]interface{} {
	entry := LogEntry(message, ctx, progress)
	if options != nil {
		entry["options"] = options
	}
	if duration > 0 {
		entry["duration"] = duration
	}
	if result != nil {
		entry["result"] = result
	}
	if metadata != nil {
		entry["metadata"] = metadata
	}
	return entry
}
