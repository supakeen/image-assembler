package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/registry"
)

// Parse transforms a raw JSON manifest (v2 only) into a typed Manifest,
// resolving module paths via the registry and fixing up runner assignments
// so each pipeline uses the runner from its build root, not its own.
func Parse(ctx context.Context, desc map[string]interface{}, reg *registry.Registry) (*Manifest, error) {
	version, _ := desc["version"].(string)
	if version != "2" {
		return nil, fmt.Errorf("unsupported manifest version: %q (only v2 is supported)", version)
	}

	m := &Manifest{
		Metadata: make(map[string]interface{}),
		byName:   make(map[string]*Pipeline),
	}

	if metadata, ok := desc["metadata"].(map[string]interface{}); ok {
		for k, v := range metadata {
			m.Metadata[k] = v
		}
	}

	sourceRefs := make(map[string]struct{})
	if sources, ok := desc["sources"].(map[string]interface{}); ok {
		for name, sdesc := range sources {
			sd, ok := sdesc.(map[string]interface{})
			if !ok {
				continue
			}
			mod, err := reg.Module("Source", name)
			if err != nil {
				return nil, fmt.Errorf("source %s: %w", name, err)
			}

			items, _ := sd["items"].(map[string]interface{})
			if items == nil {
				items = make(map[string]interface{})
			}
			options, _ := sd["options"].(map[string]interface{})
			if options == nil {
				options = make(map[string]interface{})
			}

			m.Sources = append(m.Sources, Source{
				Type:     name,
				ExecPath: mod.Path,
				Items:    items,
				Options:  options,
			})

			for ref := range items {
				sourceRefs[ref] = struct{}{}
			}
		}
	}

	if pipelines, ok := desc["pipelines"].([]interface{}); ok {
		for _, pdesc := range pipelines {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			pd, ok := pdesc.(map[string]interface{})
			if !ok {
				continue
			}
			if err := parsePipeline(pd, reg, m, sourceRefs); err != nil {
				return nil, err
			}
		}
	}

	if err := fixupRunners(m, reg); err != nil {
		return nil, err
	}

	ilog.FromContext(ctx).Debug("manifest parsed", "pipelines", len(m.Pipelines), "sources", len(m.Sources))

	return m, nil
}

func parsePipeline(desc map[string]interface{}, reg *registry.Registry, m *Manifest, sourceRefs map[string]struct{}) error {
	name, _ := desc["name"].(string)
	if name == "" {
		return fmt.Errorf("pipeline missing name")
	}

	var build *string
	if buildRef, ok := desc["build"].(string); ok {
		if strings.HasPrefix(buildRef, "name:") {
			resolved, err := resolveRef(buildRef, m)
			if err != nil {
				return fmt.Errorf("pipeline %s: %w", name, err)
			}
			build = &resolved
		} else {
			build = &buildRef
		}
	}

	var runnerName string
	if rn, ok := desc["runner"].(string); ok {
		runnerName = rn
	}

	var runner Runner
	if runnerName != "" {
		ri, err := reg.DetectRunner(runnerName)
		if err != nil {
			return fmt.Errorf("pipeline %s: runner %s: %w", name, runnerName, err)
		}
		runner = Runner{Name: runnerName, Path: ri.Path}
	} else {
		ri, err := reg.DetectHostRunner()
		if err != nil {
			return fmt.Errorf("pipeline %s: detecting host runner: %w", name, err)
		}
		runner = Runner{Path: ri.Path}
	}

	var sourceEpoch *int
	if se, ok := desc["source-epoch"]; ok {
		val, err := toInt(se)
		if err != nil {
			return fmt.Errorf("pipeline %s: source-epoch: %w", name, err)
		}
		sourceEpoch = &val
	}

	p := Pipeline{
		Name:        name,
		Build:       build,
		Runner:      runner,
		SourceEpoch: sourceEpoch,
	}
	m.Pipelines = append(m.Pipelines, p)
	m.byName[name] = &m.Pipelines[len(m.Pipelines)-1]

	if stages, ok := desc["stages"].([]interface{}); ok {
		for _, sdesc := range stages {
			sd, ok := sdesc.(map[string]interface{})
			if !ok {
				continue
			}
			if err := parseStage(sd, reg, m.byName[name], m, sourceRefs); err != nil {
				return fmt.Errorf("pipeline %s: %w", name, err)
			}
		}
	}

	return nil
}

func parseStage(desc map[string]interface{}, reg *registry.Registry, p *Pipeline, m *Manifest, sourceRefs map[string]struct{}) error {
	stageType, _ := desc["type"].(string)
	if stageType == "" {
		return fmt.Errorf("stage missing type")
	}

	mod, err := reg.Module("Stage", stageType)
	if err != nil {
		return fmt.Errorf("stage %s: %w", stageType, err)
	}

	options, _ := desc["options"].(map[string]interface{})

	stage := p.AddStage(stageType, mod.Path, mod.Caps, options, p.SourceEpoch)

	if devs, ok := desc["devices"].(map[string]interface{}); ok {
		sorted, err := sortDevices(devs)
		if err != nil {
			return fmt.Errorf("stage %s: %w", stageType, err)
		}
		for _, devName := range sorted {
			dd := devs[devName].(map[string]interface{})
			if err := loadDevice(devName, dd, reg, stage); err != nil {
				return fmt.Errorf("stage %s: %w", stageType, err)
			}
		}
	}

	if inputs, ok := desc["inputs"].(map[string]interface{}); ok {
		for inputName, idesc := range inputs {
			id, ok := idesc.(map[string]interface{})
			if !ok {
				continue
			}
			if err := loadInput(inputName, id, reg, stage, m, sourceRefs); err != nil {
				return fmt.Errorf("stage %s: %w", stageType, err)
			}
		}
	}

	if mounts, ok := desc["mounts"].([]interface{}); ok {
		for _, mdesc := range mounts {
			md, ok := mdesc.(map[string]interface{})
			if !ok {
				continue
			}
			if err := loadMount(md, reg, stage); err != nil {
				return fmt.Errorf("stage %s: %w", stageType, err)
			}
		}
	}

	return nil
}

func sortDevices(devices map[string]interface{}) ([]string, error) {
	result := make([]string, 0, len(devices))
	todo := make([]string, 0, len(devices))
	for name := range devices {
		todo = append(todo, name)
	}

	for len(todo) > 0 {
		before := len(todo)
		next := todo[:0]
		for _, name := range todo {
			dd, ok := devices[name].(map[string]interface{})
			if !ok {
				result = append(result, name)
				continue
			}
			parent, hasParent := dd["parent"].(string)
			if !hasParent || inSlice(result, parent) {
				result = append(result, name)
			} else {
				next = append(next, name)
			}
		}
		todo = next
		if len(todo) == before {
			return nil, fmt.Errorf("device dependency cycle: %v", todo)
		}
	}
	return result, nil
}

func inSlice(s []string, val string) bool {
	for _, v := range s {
		if v == val {
			return true
		}
	}
	return false
}

func loadDevice(name string, desc map[string]interface{}, reg *registry.Registry, stage *Stage) error {
	devType, _ := desc["type"].(string)
	if devType == "" {
		return fmt.Errorf("device %s missing type", name)
	}

	mod, err := reg.Module("Device", devType)
	if err != nil {
		return fmt.Errorf("device %s: %w", name, err)
	}

	options, _ := desc["options"].(map[string]interface{})
	if options == nil {
		options = make(map[string]interface{})
	}

	var parent *Device
	if parentName, ok := desc["parent"].(string); ok {
		p, exists := stage.Devices.Get(parentName)
		if !exists {
			return fmt.Errorf("device %s: parent %s not found", name, parentName)
		}
		parent = p
	}

	dev := &Device{
		Name:     name,
		Type:     devType,
		ExecPath: mod.Path,
		Parent:   parent,
		Options:  options,
	}
	stage.Devices.Set(name, dev)
	return nil
}

func loadInput(name string, desc map[string]interface{}, reg *registry.Registry, stage *Stage, m *Manifest, sourceRefs map[string]struct{}) error {
	inputType, _ := desc["type"].(string)
	if inputType == "" {
		return fmt.Errorf("input %s missing type", name)
	}

	mod, err := reg.Module("Input", inputType)
	if err != nil {
		return fmt.Errorf("input %s: %w", name, err)
	}

	origin, _ := desc["origin"].(string)
	options, _ := desc["options"].(map[string]interface{})
	if options == nil {
		options = make(map[string]interface{})
	}

	ip := &Input{
		Name:     name,
		Type:     inputType,
		ExecPath: mod.Path,
		Origin:   origin,
		Refs:     make(map[string]interface{}),
		Options:  options,
	}

	refs := desc["references"]
	switch r := refs.(type) {
	case map[string]interface{}:
		if origin == "org.osbuild.pipeline" {
			for ref, opts := range r {
				resolved := ref
				if strings.HasPrefix(ref, "name:") {
					var err error
					resolved, err = resolveRef(ref, m)
					if err != nil {
						return fmt.Errorf("input %s: %w", name, err)
					}
				}
				ip.Refs[resolved] = opts
			}
		} else {
			for ref, opts := range r {
				ip.Refs[ref] = opts
			}
		}
	case []interface{}:
		for _, item := range r {
			switch v := item.(type) {
			case string:
				ref := v
				if origin == "org.osbuild.pipeline" && strings.HasPrefix(ref, "name:") {
					var err error
					ref, err = resolveRef(v, m)
					if err != nil {
						return fmt.Errorf("input %s: %w", name, err)
					}
				}
				ip.Refs[ref] = map[string]interface{}{}
			case map[string]interface{}:
				refID, _ := v["id"].(string)
				refOpts, _ := v["options"].(map[string]interface{})
				if refOpts == nil {
					refOpts = make(map[string]interface{})
				}
				if origin == "org.osbuild.pipeline" && strings.HasPrefix(refID, "name:") {
					var err error
					refID, err = resolveRef(refID, m)
					if err != nil {
						return fmt.Errorf("input %s: %w", name, err)
					}
				}
				ip.Refs[refID] = refOpts
			}
		}
	}

	stage.Inputs.Set(name, ip)
	return nil
}

func loadMount(desc map[string]interface{}, reg *registry.Registry, stage *Stage) error {
	mountType, _ := desc["type"].(string)
	if mountType == "" {
		return fmt.Errorf("mount missing type")
	}

	name, _ := desc["name"].(string)
	if name == "" {
		return fmt.Errorf("mount missing name")
	}

	mod, err := reg.Module("Mount", mountType)
	if err != nil {
		return fmt.Errorf("mount %s: %w", name, err)
	}

	options, _ := desc["options"].(map[string]interface{})
	if options == nil {
		options = make(map[string]interface{})
	}

	target, _ := desc["target"].(string)

	var partition *int
	if p, ok := desc["partition"]; ok {
		val, err := toInt(p)
		if err != nil {
			return fmt.Errorf("mount %s: partition: %w", name, err)
		}
		partition = &val
	}

	var device *Device
	if sourceName, ok := desc["source"].(string); ok {
		d, exists := stage.Devices.Get(sourceName)
		if !exists {
			return fmt.Errorf("mount %s: source device %s not found", name, sourceName)
		}
		device = d
	}

	mnt := &Mount{
		Name:      name,
		Type:      mountType,
		ExecPath:  mod.Path,
		Device:    device,
		Partition: partition,
		Target:    target,
		Options:   options,
	}
	stage.Mounts.Set(name, mnt)
	return nil
}

func resolveRef(ref string, m *Manifest) (string, error) {
	name := ref[5:]
	p := m.Pipeline(name)
	if p == nil {
		return "", fmt.Errorf("pipeline %q not found", name)
	}
	return p.ID(), nil
}

func fixupRunners(m *Manifest, reg *registry.Registry) error {
	ri, err := reg.DetectHostRunner()
	if err != nil {
		return fmt.Errorf("detecting host runner: %w", err)
	}
	hostRunner := Runner{Path: ri.Path}

	runners := make(map[string]Runner)
	for i := range m.Pipelines {
		id := m.Pipelines[i].ID()
		if id != "" {
			runners[id] = m.Pipelines[i].Runner
		}
	}

	for i := range m.Pipelines {
		if m.Pipelines[i].Build == nil {
			m.Pipelines[i].Runner = hostRunner
		} else {
			if r, ok := runners[*m.Pipelines[i].Build]; ok {
				m.Pipelines[i].Runner = r
			}
		}
	}
	return nil
}

func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case json.Number:
		n, err := strconv.Atoi(val.String())
		if err != nil {
			return 0, err
		}
		return n, nil
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case int64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}
