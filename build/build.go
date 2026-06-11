// Package build orchestrates pipeline execution: running stages inside
// sandboxed containers, downloading sources via Python service processes,
// and exporting results to user-specified directories. It matches the
// execution model of Python osbuild's pipeline.py and main_cli.py.
package build

import (
	"context"
	"fmt"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/monitor"
	"github.com/supakeen/image-assembler/store"
)

// BuildPipeline executes a single pipeline's stages in order. It skips the
// pipeline entirely if its final ID is already in the store (cached), and
// resumes from the last checkpointed stage if one exists. Each stage runs in
// a bubblewrap sandbox with its own device/input/mount managers.
func BuildPipeline(ctx context.Context, p *manifest.Pipeline, st *store.Store,
	mon monitor.Monitor, libdir string, stageTimeout *int) (map[string]interface{}, error) {

	logger := ilog.FromContext(ctx)

	results := map[string]interface{}{
		"success": true,
		"name":    p.Name,
	}

	if len(p.Stages) == 0 {
		return results, nil
	}

	if st.Contains(p.ID()) {
		logger.Debug("pipeline cached, skipping", "name", p.Name, "id", p.ID())
		return results, nil
	}

	logger.Info("building pipeline", "name", p.Name, "id", p.ID(), "stages", len(p.Stages))

	var buildTree BuildTree
	if p.Build == nil {
		ht, err := st.GetHostTree(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting host tree: %w", err)
		}
		buildTree = ht
	} else {
		obj, err := st.Get(ctx, *p.Build)
		if err != nil {
			return nil, fmt.Errorf("getting build tree %s: %w", *p.Build, err)
		}
		if obj == nil {
			return nil, fmt.Errorf("build tree %s not found", *p.Build)
		}
		buildTree = obj
	}

	tree, err := st.New(ctx, p.ID())
	if err != nil {
		return nil, fmt.Errorf("creating object: %w", err)
	}
	tree.SourceEpoch = p.SourceEpoch

	// Walk stages in reverse to find last checkpoint in store.
	var todo []int
	for i := len(p.Stages) - 1; i >= 0; i-- {
		base, err := st.Get(ctx, p.Stages[i].ID())
		if err != nil {
			tree.Close()
			return nil, fmt.Errorf("checking checkpoint %s: %w", p.Stages[i].ID(), err)
		}
		if base != nil && base.Mode() == store.ModeRead {
			if err := tree.Init(ctx, base); err != nil {
				tree.Close()
				return nil, fmt.Errorf("init from checkpoint: %w", err)
			}
			break
		}
		todo = append(todo, i)
	}

	stageResults := make([]interface{}, 0)

	for j := len(todo) - 1; j >= 0; j-- {
		if err := ctx.Err(); err != nil {
			tree.Close()
			return nil, err
		}

		stage := &p.Stages[todo[j]]
		mon.Stage(stage)

		metaWriter, err := tree.Meta().Write(stage.ID())
		if err != nil {
			tree.Close()
			return nil, fmt.Errorf("opening metadata writer: %w", err)
		}

		r, err := RunStage(ctx, stage, tree.Tree(), metaWriter.Name,
			p.Runner, buildTree, st, libdir, stageTimeout, mon)

		metaWriter.Close()

		if err != nil {
			tree.Close()
			return nil, fmt.Errorf("running stage %s: %w", stage.Name(), err)
		}

		md, _ := tree.Meta().Get(r.Stage.ID())
		mon.Result(r, md)

		stageResults = append(stageResults, r)

		if !r.Success() {
			tree.Close()
			results["success"] = false
			results["stages"] = stageResults
			return results, nil
		}

		if stage.Checkpoint {
			logger.Debug("committing checkpoint", "stage", stage.Name(), "id", stage.ID())
			if err := st.Commit(ctx, tree, stage.ID()); err != nil {
				tree.Close()
				return nil, fmt.Errorf("committing checkpoint: %w", err)
			}
		}
	}

	if err := tree.Finalize(ctx); err != nil {
		tree.Close()
		return nil, fmt.Errorf("finalizing tree: %w", err)
	}

	results["stages"] = stageResults
	return results, nil
}

// Build executes pipelines in dependency order (as determined by Depsolve),
// stopping on the first failure. Returns a results map keyed by pipeline ID
// with per-pipeline stage results.
func Build(ctx context.Context, m *manifest.Manifest, st *store.Store,
	pipelines []string, mon monitor.Monitor, libdir string,
	stageTimeout *int) (map[string]interface{}, error) {

	ilog.FromContext(ctx).Info("build started", "pipeline_count", len(pipelines))

	results := map[string]interface{}{
		"success": true,
	}

	for _, name := range pipelines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		p := m.Pipeline(name)
		if p == nil {
			p = m.PipelineByID(name)
		}
		if p == nil {
			return nil, fmt.Errorf("pipeline %q not found", name)
		}

		mon.Begin(p)

		res, err := BuildPipeline(ctx, p, st, mon, libdir, stageTimeout)
		if err != nil {
			return nil, err
		}

		mon.Finish(res)

		results[p.ID()] = res

		success, _ := res["success"].(bool)
		if !success {
			results["success"] = false
			return results, nil
		}
	}

	return results, nil
}
