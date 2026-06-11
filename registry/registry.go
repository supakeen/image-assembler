// Package registry discovers and loads osbuild modules (stages, sources,
// devices, inputs, mounts, runners) from the libdir filesystem layout
// (typically /usr/lib/osbuild). Modules are loaded lazily on first access
// and cached for the lifetime of the Registry.
package registry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	ilog "github.com/supakeen/image-assembler/internal/log"
)

// ModuleDirs maps module kinds to their subdirectory names under libdir.
var ModuleDirs = map[string]string{
	"Stage":     "stages",
	"Source":    "sources",
	"Device":   "devices",
	"Input":    "inputs",
	"Mount":    "mounts",
	"Assembler": "assemblers",
}

// Module represents a discovered osbuild module (stage, source, device, etc.)
// with its metadata, JSON schemas, and capabilities loaded from the
// corresponding .meta.json sidecar file.
type Module struct {
	Name    string
	Kind    string
	Path    string
	Desc    string
	Info    string
	Caps    map[string]struct{}
	Schemas map[string]interface{}
}

// RunnerInfo identifies an osbuild runner script for a specific distro and
// version. Runners execute inside the buildroot to set up the environment
// before stages run.
type RunnerInfo struct {
	Distro  string
	Version int
	Path    string
}

// Registry provides lazy-loading access to osbuild modules under a libdir.
// It caches modules and schemas after first access so repeated lookups
// during manifest validation and build execution are free.
type Registry struct {
	path       string
	modules    map[[2]string]*Module
	schemata   map[[3]string]*Schema
	runners    []RunnerInfo
	hostRunner *RunnerInfo
	logger     *slog.Logger
}

func New(libdir string) *Registry {
	return &Registry{
		path:     libdir,
		modules:  make(map[[2]string]*Module),
		schemata: make(map[[3]string]*Schema),
		logger:   ilog.Discard(),
	}
}

func (r *Registry) SetLogger(logger *slog.Logger) {
	r.logger = logger
}

func (r *Registry) Module(kind, name string) (*Module, error) {
	key := [2]string{kind, name}
	if m, ok := r.modules[key]; ok {
		return m, nil
	}

	dir, ok := ModuleDirs[kind]
	if !ok {
		return nil, fmt.Errorf("unknown module kind: %s", kind)
	}

	modPath := filepath.Join(r.path, dir, name)
	m, err := loadModule(kind, name, modPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s %s: %w", kind, name, err)
	}

	r.modules[key] = m
	r.logger.Log(nil, ilog.LevelTrace, "registry lookup", "kind", kind, "name", name, "path", m.Path)
	return m, nil
}

func loadModule(kind, name, modPath string) (*Module, error) {
	metaPath := modPath + ".meta.json"
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Module{
				Name:    name,
				Kind:    kind,
				Path:    modPath,
				Schemas: make(map[string]interface{}),
				Caps:    make(map[string]struct{}),
			}, nil
		}
		return nil, err
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", metaPath, err)
	}

	m := &Module{
		Name:    name,
		Kind:    kind,
		Path:    modPath,
		Schemas: make(map[string]interface{}),
		Caps:    make(map[string]struct{}),
	}

	if v, ok := meta["summary"].(string); ok {
		m.Desc = v
	}
	if v, ok := meta["description"].([]interface{}); ok {
		parts := make([]string, len(v))
		for i, s := range v {
			parts[i], _ = s.(string)
		}
		m.Info = strings.Join(parts, "\n")
	}

	if v, ok := meta["schema_2"]; ok {
		m.Schemas["2"] = v
	}
	if v, ok := meta["schema"]; ok {
		m.Schemas["1"] = v
	}

	if caps, ok := meta["capabilities"].([]interface{}); ok {
		for _, c := range caps {
			if s, ok := c.(string); ok {
				m.Caps[s] = struct{}{}
			}
		}
	}

	return m, nil
}

func (m *Module) BuildSchema(version string) map[string]interface{} {
	raw, ok := m.Schemas[version]
	if !ok {
		return nil
	}
	rawMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}

	switch m.Kind {
	case "Stage", "Assembler":
		return m.buildStageSchema(rawMap, version)
	case "Device":
		return m.buildDeviceSchema(rawMap)
	case "Mount":
		return m.buildMountSchema(rawMap)
	default:
		return m.buildGenericSchema(rawMap)
	}
}

func (m *Module) buildStageSchema(raw map[string]interface{}, version string) map[string]interface{} {
	schema := map[string]interface{}{
		"title":                fmt.Sprintf("Pipeline %s", m.Kind),
		"type":                 "object",
		"additionalProperties": false,
	}

	props := map[string]interface{}{
		"type": map[string]interface{}{
			"enum": []interface{}{m.Name},
		},
		"mounts": map[string]interface{}{
			"type": "array",
		},
		"devices": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		},
	}

	if version == "2" {
		if opts, ok := raw["options"]; ok {
			props["options"] = opts
		}
		for k, v := range raw {
			if k != "options" {
				props[k] = v
			}
		}
	} else {
		props["options"] = raw
	}

	schema["properties"] = props
	schema["required"] = []interface{}{"type"}

	hoistDefinitions(schema, props)

	return schema
}

func (m *Module) buildDeviceSchema(raw map[string]interface{}) map[string]interface{} {
	schema := map[string]interface{}{
		"additionalProperties": true,
	}
	props := map[string]interface{}{
		"type": map[string]interface{}{
			"enum": []interface{}{m.Name},
		},
	}
	if opts, ok := raw["options"]; ok {
		props["options"] = opts
	}
	schema["properties"] = props
	return schema
}

func (m *Module) buildMountSchema(raw map[string]interface{}) map[string]interface{} {
	schema := make(map[string]interface{})
	for k, v := range raw {
		schema[k] = v
	}
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		props["type"] = map[string]interface{}{
			"enum": []interface{}{m.Name},
		}
	} else {
		schema["properties"] = map[string]interface{}{
			"type": map[string]interface{}{
				"enum": []interface{}{m.Name},
			},
		}
	}
	return schema
}

func (m *Module) buildGenericSchema(raw map[string]interface{}) map[string]interface{} {
	schema := make(map[string]interface{})
	for k, v := range raw {
		schema[k] = v
	}
	return schema
}

func hoistDefinitions(schema, props map[string]interface{}) {
	if defs, ok := props["definitions"]; ok {
		schema["definitions"] = defs
		delete(props, "definitions")
	}
	if opts, ok := props["options"].(map[string]interface{}); ok {
		if defs, ok := opts["definitions"]; ok {
			schema["definitions"] = defs
			delete(opts, "definitions")
		}
	}
}

func (r *Registry) Schema(kind, name, version string) *Schema {
	key := [3]string{kind, name, version}
	if s, ok := r.schemata[key]; ok {
		return s
	}

	var data map[string]interface{}

	if kind == "Manifest" {
		schemaFile := filepath.Join(r.path, "schemas", fmt.Sprintf("osbuild%s.json", version))
		raw, err := os.ReadFile(schemaFile)
		if err != nil {
			s := &Schema{Name: fmt.Sprintf("%s/%s", kind, version)}
			r.schemata[key] = s
			return s
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			s := &Schema{Name: fmt.Sprintf("%s/%s", kind, version)}
			r.schemata[key] = s
			return s
		}
	} else {
		mod, err := r.Module(kind, name)
		if err != nil || mod == nil {
			s := &Schema{Name: fmt.Sprintf("%s/%s/%s", kind, name, version)}
			r.schemata[key] = s
			return s
		}
		data = mod.BuildSchema(version)
	}

	s := &Schema{Data: data, Name: fmt.Sprintf("%s/%s/%s", kind, name, version)}
	r.schemata[key] = s
	return s
}

func (r *Registry) loadRunners() {
	if r.runners != nil {
		return
	}
	r.runners = []RunnerInfo{}

	runnersDir := filepath.Join(r.path, "runners")
	entries, err := os.ReadDir(runnersDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		distro, version := parseRunnerName(name)
		r.runners = append(r.runners, RunnerInfo{
			Distro:  distro,
			Version: version,
			Path:    filepath.Join(runnersDir, name),
		})
	}

	sort.Slice(r.runners, func(i, j int) bool {
		if r.runners[i].Distro != r.runners[j].Distro {
			return r.runners[i].Distro < r.runners[j].Distro
		}
		return r.runners[i].Version < r.runners[j].Version
	})
}

func parseRunnerName(name string) (string, int) {
	i := len(name)
	for i > 0 && unicode.IsDigit(rune(name[i-1])) {
		i--
	}
	if i == len(name) {
		return name, 0
	}
	distro := name[:i]
	version, _ := strconv.Atoi(name[i:])
	return distro, version
}

func (r *Registry) ListRunners(distro string) []RunnerInfo {
	r.loadRunners()
	if distro == "" {
		return r.runners
	}
	var result []RunnerInfo
	for _, ri := range r.runners {
		if ri.Distro == distro {
			result = append(result, ri)
		}
	}
	return result
}

// DetectRunner finds the best runner for the given OS name (e.g.
// "org.osbuild.fedora40") by selecting the highest-versioned runner
// that doesn't exceed the target version.
func (r *Registry) DetectRunner(name string) (*RunnerInfo, error) {
	distro, version := parseRunnerName(name)
	runners := r.ListRunners(distro)

	for i := len(runners) - 1; i >= 0; i-- {
		if runners[i].Version <= version {
			r.logger.Log(nil, ilog.LevelTrace, "runner detected", "name", name, "version", runners[i].Version, "path", runners[i].Path)
			return &runners[i], nil
		}
	}

	return nil, fmt.Errorf("no runner found for %s", name)
}

func (r *Registry) DetectHostRunner() (*RunnerInfo, error) {
	if r.hostRunner != nil {
		return r.hostRunner, nil
	}

	osName, err := DescribeOS()
	if err != nil {
		return nil, fmt.Errorf("detecting host OS: %w", err)
	}

	ri, err := r.DetectRunner("org.osbuild." + osName)
	if err != nil {
		return nil, err
	}

	r.hostRunner = ri
	return ri, nil
}
