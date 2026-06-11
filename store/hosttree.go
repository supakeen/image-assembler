package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	ilog "github.com/supakeen/image-assembler/internal/log"
)

// HostTree provides a minimal read-only view of the host filesystem for
// stages that need host binaries (e.g. /usr) or container config
// (/etc/containers). Each directory is bind-mounted privately so stage
// modifications can't leak back to the host.
type HostTree struct {
	root   string
	logger *slog.Logger
}

func NewHostTree(ctx context.Context, s *Store) (*HostTree, error) {
	dir, err := os.MkdirTemp(s.tmp, "host")
	if err != nil {
		return nil, fmt.Errorf("creating host tree dir: %w", err)
	}

	for _, sub := range []string{"usr", "etc/containers"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("creating host tree subdirectory: %w", err)
		}
	}

	mounts := []struct{ src, dst string }{
		{dir, dir},
		{"/usr", filepath.Join(dir, "usr")},
		{"/etc/containers", filepath.Join(dir, "etc", "containers")},
	}

	for _, m := range mounts {
		if _, err := os.Stat(m.src); os.IsNotExist(err) {
			continue
		}
		cmd := exec.CommandContext(ctx, "mount", "--rbind", "--make-rprivate", m.src, m.dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			exec.Command("umount", "-R", dir).Run()
			os.RemoveAll(dir)
			return nil, fmt.Errorf("bind mount %s: %s: %w", m.src, out, err)
		}
	}

	logger := ilog.FromContext(ctx)
	logger.Info("host tree created", "root", dir)

	return &HostTree{root: dir, logger: logger}, nil
}

func (h *HostTree) Tree() string {
	return h.root
}

func (h *HostTree) Close() {
	if h.root == "" {
		return
	}
	h.cleanupMounts()
	os.RemoveAll(h.root)
	h.root = ""
}

func (h *HostTree) cleanupMounts() {
	if out, err := exec.Command("sync", "-f", h.root).CombinedOutput(); err != nil {
		h.logger.Warn("sync failed during cleanup", "root", h.root, "error", err, "output", string(out))
	}
	if out, err := exec.Command("umount", "-R", h.root).CombinedOutput(); err != nil {
		h.logger.Warn("umount failed during cleanup", "root", h.root, "error", err, "output", string(out))
	}
}
