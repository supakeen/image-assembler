package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/store"
)

// Export copies a built pipeline's tree from the object store to the user's
// output directory. This is the final step that produces user-visible
// artifacts (disk images, tarballs, etc.) from the store's internal layout.
func Export(ctx context.Context, nameOrID string, outputDir string,
	st *store.Store, m *manifest.Manifest) error {

	ilog.FromContext(ctx).Info("exporting pipeline", "name", nameOrID, "destination", outputDir)

	p := m.Pipeline(nameOrID)
	if p == nil {
		p = m.PipelineByID(nameOrID)
	}
	if p == nil {
		return fmt.Errorf("pipeline %q not found", nameOrID)
	}

	obj, err := st.Get(ctx, p.ID())
	if err != nil {
		return fmt.Errorf("getting pipeline object: %w", err)
	}
	if obj == nil {
		return fmt.Errorf("pipeline %q not in store", nameOrID)
	}

	dest := filepath.Join(outputDir, nameOrID)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("creating export directory: %w", err)
	}

	skipPreserveOwner := os.Getenv("OSBUILD_EXPORT_FORCE_NO_PRESERVE_OWNER") == "1"
	return obj.Export(ctx, dest, skipPreserveOwner)
}
