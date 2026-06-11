package manifest

import (
	"context"

	ilog "github.com/supakeen/image-assembler/internal/log"
)

// StoreChecker abstracts the object store for dependency resolution, letting
// Depsolve skip pipelines whose output is already cached without importing
// the full store package.
type StoreChecker interface {
	Contains(id string) bool
}

// Depsolve returns a build-order list of pipeline names needed to produce
// the given targets, skipping any whose output is already in the store.
// It walks build dependencies and cross-pipeline input references,
// performing a reverse topological sort so dependencies build first.
func (m *Manifest) Depsolve(ctx context.Context, store StoreChecker, targets []string) ([]string, error) {
	var check []*Pipeline
	for _, t := range targets {
		p := m.Pipeline(t)
		if p == nil {
			p = m.PipelineByID(t)
		}
		if p != nil {
			check = append(check, p)
		}
	}

	type entry struct {
		id string
		pl *Pipeline
	}

	buildKeys := make([]string, 0)
	buildMap := make(map[string]*Pipeline)

	for len(check) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		pl := check[len(check)-1]
		check = check[:len(check)-1]

		if pl == nil {
			continue
		}

		id := pl.ID()
		if id == "" || store.Contains(id) {
			continue
		}

		if _, exists := buildMap[id]; exists {
			newKeys := make([]string, 0, len(buildKeys))
			for _, k := range buildKeys {
				if k != id {
					newKeys = append(newKeys, k)
				}
			}
			newKeys = append(newKeys, id)
			buildKeys = newKeys
		} else {
			buildKeys = append(buildKeys, id)
		}
		buildMap[id] = pl

		if pl.Build != nil {
			bp := m.PipelineByID(*pl.Build)
			check = append(check, bp)
		}

		for i := len(pl.Stages) - 1; i >= 0; i-- {
			stage := &pl.Stages[i]
			if store.Contains(stage.ID()) {
				break
			}
			for _, dep := range stage.Dependencies() {
				dp := m.PipelineByID(dep)
				if dp != nil {
					check = append(check, dp)
				}
			}
		}
	}

	result := make([]string, len(buildKeys))
	for i, k := range buildKeys {
		result[len(buildKeys)-1-i] = buildMap[k].Name
	}

	ilog.FromContext(ctx).Debug("depsolve result", "order", result)
	return result, nil
}
