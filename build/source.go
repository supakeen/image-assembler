package build

import (
	"context"
	"fmt"
	"path/filepath"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/monitor"
	"github.com/supakeen/image-assembler/service"
	"github.com/supakeen/image-assembler/store"
)

// DownloadResult adapts a source download outcome to the monitor.Result
// interface so source progress appears in monitor output alongside stages.
type DownloadResult struct {
	source  *manifest.Source
	success bool
	output  string
}

func (r *DownloadResult) ResultName() string   { return r.source.ModuleName() }
func (r *DownloadResult) ResultID() string     { return r.source.ID() }
func (r *DownloadResult) Success() bool        { return r.success }
func (r *DownloadResult) ResultOutput() string { return r.output }
func (r *DownloadResult) AsDict() map[string]interface{} {
	return map[string]interface{}{
		"name":    r.source.ModuleName(),
		"id":      r.source.ID(),
		"success": r.success,
		"output":  r.output,
	}
}

// DownloadSource starts the Python source service process and calls its
// "download" method with the source items and cache directory.
func DownloadSource(ctx context.Context, src *manifest.Source, mgr *service.Manager, st *store.Store) error {
	ilog.FromContext(ctx).Debug("downloading source", "type", src.Type)
	cache := filepath.Join(st.Root(), "sources")

	args := map[string]interface{}{
		"items":     src.Items,
		"options":   src.Options,
		"cache":     cache,
		"output":    nil,
		"checksums": []interface{}{},
	}

	client, err := mgr.Start(ctx, "source/"+src.Type, src.ExecPath, nil)
	if err != nil {
		return fmt.Errorf("starting source service %s: %w", src.Type, err)
	}

	_, err = client.Call(ctx, "download", args)
	if err != nil {
		return fmt.Errorf("downloading source %s: %w", src.Type, err)
	}

	return nil
}

// DownloadAll fetches all sources in the manifest sequentially, reporting
// progress to the monitor. It stops on the first download failure.
func DownloadAll(ctx context.Context, m *manifest.Manifest, st *store.Store, libdir string, mon monitor.Monitor) error {
	logger := ilog.FromContext(ctx)
	logger.Info("downloading sources", "count", len(m.Sources))

	mgr := service.NewManager(libdir)
	mgr.SetLogger(logger)
	defer mgr.Close()

	for i := range m.Sources {
		if err := ctx.Err(); err != nil {
			return err
		}

		src := &m.Sources[i]
		dr := &DownloadResult{source: src, success: true}
		mon.Begin(src)

		if err := DownloadSource(ctx, src, mgr, st); err != nil {
			dr.success = false
			dr.output = err.Error()
			mon.Result(dr, nil)
			return fmt.Errorf("source %s: %w", src.Type, err)
		}

		mon.Result(dr, nil)
		mon.Finish(map[string]interface{}{"name": src.ModuleName()})
	}

	return nil
}
