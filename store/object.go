package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	ilog "github.com/supakeen/image-assembler/internal/log"
)

// ObjectMode tracks whether an object is writable (in-progress build) or
// read-only (committed or finalized). Objects start in ModeWrite and
// transition to ModeRead via Finalize or Commit — never the reverse.
type ObjectMode int

const (
	ModeRead  ObjectMode = 0
	ModeWrite ObjectMode = 1
)

// Object represents a build artifact in the store. In ModeWrite it lives
// under stage/ and can be modified; in ModeRead it is either committed
// to objects/ or finalized in-memory as a floating object.
type Object struct {
	id          string
	mode        ObjectMode
	path        string
	stagePath   string
	meta        *Metadata
	SourceEpoch *int
}

func (o *Object) ID() string       { return o.id }
func (o *Object) Mode() ObjectMode { return o.mode }
func (o *Object) Path() string     { return o.path }
func (o *Object) Tree() string     { return filepath.Join(o.path, "tree") }
func (o *Object) Meta() *Metadata  { return o.meta }

func (o *Object) Created() (int, error) {
	info, err := o.meta.Get("info")
	if err != nil {
		return 0, fmt.Errorf("reading creation time: %w", err)
	}
	if info == nil {
		return 0, fmt.Errorf("no info metadata")
	}
	v, ok := info["created"]
	if !ok {
		return 0, fmt.Errorf("no created field in info metadata")
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("unexpected created type %T", v)
	}
}

// Init copies the contents of a base object into this writable object,
// enabling checkpoint-based resumption: if a previous stage was cached,
// the pipeline resumes from that point instead of rebuilding from scratch.
func (o *Object) Init(ctx context.Context, base *Object) error {
	if o.mode != ModeWrite {
		return fmt.Errorf("cannot init: object not in write mode")
	}
	ilog.FromContext(ctx).Debug("object init from base", "id", o.id, "base_id", base.id)
	cmd := exec.CommandContext(ctx, "cp", "--reflink=auto", "-a",
		base.Path()+"/.", o.Path())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("init copy: %s: %w", out, err)
	}
	return nil
}

func (o *Object) ClampMtime(ctx context.Context) error {
	if o.SourceEpoch == nil {
		return nil
	}
	created, err := o.Created()
	if err != nil {
		return err
	}
	return ClampMtime(ctx, o.Tree(), created, *o.SourceEpoch)
}

// Finalize clamps mtimes for reproducibility and transitions the object
// to ModeRead. Called at the end of a pipeline build before the result
// is used or exported.
func (o *Object) Finalize(ctx context.Context) error {
	if o.mode != ModeWrite {
		return nil
	}
	if err := o.ClampMtime(ctx); err != nil {
		return fmt.Errorf("finalize: %w", err)
	}
	o.mode = ModeRead
	ilog.FromContext(ctx).Debug("object finalized", "id", o.id)
	return nil
}

func (o *Object) Export(ctx context.Context, toDir string, skipPreserveOwner bool) error {
	ilog.FromContext(ctx).Info("object exported", "id", o.id, "destination", toDir)
	args := []string{"--reflink=auto", "-a"}
	if skipPreserveOwner {
		args = append(args, "--no-preserve=ownership")
	}
	args = append(args, o.Tree()+"/.", toDir)
	cmd := exec.CommandContext(ctx, "cp", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("export: %s: %w", out, err)
	}
	return nil
}

func (o *Object) Close() {
	if o.stagePath != "" && o.mode == ModeWrite {
		os.RemoveAll(o.stagePath)
	}
}

func (o *Object) ToDict(cacheRoot string) map[string]interface{} {
	relPath, _ := filepath.Rel(cacheRoot, o.path)
	return map[string]interface{}{
		"mode": int(o.mode),
		"id":   o.id,
		"path": relPath,
	}
}

func ObjectFromDict(data map[string]interface{}, cacheRoot string) (*Object, error) {
	id, _ := data["id"].(string)
	relPath, _ := data["path"].(string)
	modeVal, _ := data["mode"].(float64)

	absPath := filepath.Join(cacheRoot, relPath)
	meta, err := NewMetadata(absPath, "meta")
	if err != nil {
		return nil, fmt.Errorf("loading floating object metadata: %w", err)
	}

	return &Object{
		id:   id,
		mode: ObjectMode(int(modeVal)),
		path: absPath,
		meta: meta,
	}, nil
}
