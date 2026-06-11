// Package manifest parses and represents osbuild v2 manifests. A manifest
// describes a DAG of pipelines, each containing an ordered list of stages
// that transform a filesystem tree. The package handles parsing from JSON,
// content-addressed ID computation (matching Python's checksumming for cache
// compatibility), dependency resolution, schema validation, and
// round-tripping back to JSON via Describe.
package manifest

import "path/filepath"

// Manifest is the top-level container: an ordered list of pipelines, a set
// of sources to download, and arbitrary metadata passed through to results.
type Manifest struct {
	Metadata  map[string]interface{}
	Pipelines []Pipeline
	Sources   []Source
	byName    map[string]*Pipeline
}

func (m *Manifest) Pipeline(name string) *Pipeline {
	return m.byName[name]
}

func (m *Manifest) PipelineByID(id string) *Pipeline {
	for i := range m.Pipelines {
		if m.Pipelines[i].ID() == id {
			return &m.Pipelines[i]
		}
	}
	return nil
}

// Pipeline groups stages that build a single filesystem tree. A pipeline
// may reference another pipeline as its Build root (the container in which
// stages execute) and carries a Runner that selects the OS-specific
// entry point script inside that build root.
type Pipeline struct {
	Name        string
	Build       *string
	Runner      Runner
	Stages      []Stage
	SourceEpoch *int
}

func (p *Pipeline) ModuleName() string {
	return p.Name
}

func (p *Pipeline) AddStage(typ, execPath string, caps map[string]struct{}, options map[string]interface{}, sourceEpoch *int) *Stage {
	if options == nil {
		options = make(map[string]interface{})
	}
	s := Stage{
		Type:        typ,
		ExecPath:    execPath,
		Caps:        caps,
		Build:       p.Build,
		Base:        p.idPtr(),
		Options:     options,
		SourceEpoch: sourceEpoch,
		Inputs:      NewOrderedMap[*Input](),
		Devices:     NewOrderedMap[*Device](),
		Mounts:      NewOrderedMap[*Mount](),
	}
	p.Stages = append(p.Stages, s)
	return &p.Stages[len(p.Stages)-1]
}

func (p *Pipeline) idPtr() *string {
	if len(p.Stages) == 0 {
		return nil
	}
	id := p.Stages[len(p.Stages)-1].ID()
	return &id
}

func (p *Pipeline) ID() string {
	if len(p.Stages) == 0 {
		return ""
	}
	return p.Stages[len(p.Stages)-1].ID()
}

// Stage is a single transformation step. Its content-addressed ID is a
// SHA-256 hash of its type, options, base, build ref, inputs, and mounts,
// computed identically to Python osbuild so cached objects are interchangeable.
type Stage struct {
	Type        string
	ExecPath    string
	Caps        map[string]struct{}
	Sources     map[string]interface{}
	Build       *string
	Base        *string
	Options     map[string]interface{}
	SourceEpoch *int
	Checkpoint  bool
	Inputs      OrderedMap[*Input]
	Devices     OrderedMap[*Device]
	Mounts      OrderedMap[*Mount]
}

func (s *Stage) Name() string {
	return s.Type
}

// Dependencies returns pipeline IDs that this stage's inputs reference,
// enabling Depsolve to discover cross-pipeline edges in the build graph.
func (s *Stage) Dependencies() []string {
	var deps []string
	s.Inputs.Range(func(_ string, ip *Input) bool {
		if ip.Origin == "org.osbuild.pipeline" {
			for ref := range ip.Refs {
				deps = append(deps, ref)
			}
		}
		return true
	})
	return deps
}

// Runner identifies the OS-specific wrapper script (e.g. runners/org.osbuild.fedora42)
// that sets up the execution environment before invoking a stage binary.
type Runner struct {
	Name string
	Path string
}

func (r *Runner) Exec() string {
	return filepath.Base(r.Path)
}

// Input provides data to a stage from an external origin (another pipeline's
// tree, downloaded source files, etc.). The Origin field determines how Refs
// are resolved: "org.osbuild.pipeline" refs point to pipeline IDs, while
// source-backed origins use content hashes.
type Input struct {
	Name     string
	Type     string
	ExecPath string
	Origin   string
	Refs     map[string]interface{}
	Options  map[string]interface{}
}

// Device represents a block device (loopback, LVM, etc.) attached to a
// stage. Devices can form a parent chain (e.g. a partition on a loop device)
// and are opened in topological order during stage execution.
type Device struct {
	Name     string
	Type     string
	ExecPath string
	Parent   *Device
	Options  map[string]interface{}
}

// Mount attaches a filesystem from a Device into the stage's tree at Target.
// The Partition field selects a partition number when the device is a whole disk.
type Mount struct {
	Name      string
	Type      string
	ExecPath  string
	Device    *Device
	Partition *int
	Target    string
	Options   map[string]interface{}
}

// Source describes content to download before the build starts (RPM packages,
// container images, files by checksum, etc.). Each source type is backed by
// a Python service process that handles the actual download.
type Source struct {
	Type     string
	ExecPath string
	Items    map[string]interface{}
	Options  map[string]interface{}
}

func (s *Source) ModuleName() string {
	return "source " + s.Type
}
