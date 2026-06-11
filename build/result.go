package build

import "github.com/supakeen/image-assembler/manifest"

// BuildResult captures the outcome of a single stage execution, satisfying
// the monitor.Result interface so stage results can be reported to any monitor.
type BuildResult struct {
	Stage      *manifest.Stage
	ReturnCode int
	Output     string
	Error      map[string]interface{}
}

func (r *BuildResult) ResultName() string { return r.Stage.Name() }
func (r *BuildResult) ResultID() string   { return r.Stage.ID() }
func (r *BuildResult) Success() bool      { return r.ReturnCode == 0 }
func (r *BuildResult) ResultOutput() string { return r.Output }

func (r *BuildResult) AsDict() map[string]interface{} {
	return map[string]interface{}{
		"name":    r.Stage.Name(),
		"id":      r.Stage.ID(),
		"success": r.ReturnCode == 0,
		"output":  r.Output,
		"error":   r.Error,
	}
}
