package manifest

import "path/filepath"

// MarkCheckpoints flags stages whose IDs or names match any of the given glob
// patterns (filepath.Match syntax). Checkpointed stages are committed to the
// store mid-pipeline, allowing future builds to resume from that point.
func (m *Manifest) MarkCheckpoints(patterns []string) map[string]struct{} {
	selected := make(map[string]struct{})

	for i := range m.Pipelines {
		p := &m.Pipelines[i]
		if len(p.Stages) == 0 {
			continue
		}

		if matching(p.Name, patterns) {
			last := &p.Stages[len(p.Stages)-1]
			last.Checkpoint = true
			selected[last.ID()] = struct{}{}
		}

		for j := range p.Stages {
			s := &p.Stages[j]
			if matching(s.ID(), patterns) || matching(s.Name(), patterns) {
				s.Checkpoint = true
				selected[s.ID()] = struct{}{}
			}
		}
	}

	return selected
}

func matching(haystack string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, haystack); ok {
			return true
		}
	}
	return false
}
