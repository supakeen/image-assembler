package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/manifest"
)

// MountManager mounts filesystems for a stage by calling their Python
// service processes. It resolves device paths via DeviceManager and
// returns paths relative to the mounts root directory.
type MountManager struct {
	devices *DeviceManager
	root    string
	mounts  map[string]interface{}
}

func NewMountManager(devMgr *DeviceManager, root string) *MountManager {
	return &MountManager{
		devices: devMgr,
		root:    root,
		mounts:  make(map[string]interface{}),
	}
}

func (m *MountManager) Mount(ctx context.Context, mnt *manifest.Mount) (map[string]interface{}, error) {
	logger := ilog.FromContext(ctx)
	source := m.devices.AbsPath(mnt.Device)
	relpath := m.devices.RelPath(mnt.Device)

	// Prefer /dev path if it exists on host (for grub2-install compatibility)
	if relpath != "" {
		hostDevPath := filepath.Join("/dev", relpath)
		if _, err := os.Stat(hostDevPath); err == nil {
			source = hostDevPath
		}
	}

	if source != "" && mnt.Partition != nil {
		source = fmt.Sprintf("%sp%d", source, *mnt.Partition)
	}

	args := map[string]interface{}{
		"source":  source,
		"target":  mnt.Target,
		"root":    m.root,
		"tree":    m.devices.TreePath,
		"options": mnt.Options,
	}

	client, err := m.devices.services.Start(ctx, "mount/"+mnt.Name, mnt.ExecPath, nil)
	if err != nil {
		return nil, fmt.Errorf("starting mount service %s: %w", mnt.Name, err)
	}

	reply, err := client.Call(ctx, "mount", args)
	if err != nil {
		return nil, fmt.Errorf("calling mount %s: %w", mnt.Name, err)
	}

	if reply == nil {
		m.mounts[mnt.Name] = map[string]interface{}{}
		return map[string]interface{}{}, nil
	}

	pathStr, ok := reply.(string)
	if !ok {
		if replyMap, ok := reply.(map[string]interface{}); ok {
			m.mounts[mnt.Name] = replyMap
			return replyMap, nil
		}
		m.mounts[mnt.Name] = map[string]interface{}{}
		return map[string]interface{}{}, nil
	}

	if pathStr == "" {
		m.mounts[mnt.Name] = map[string]interface{}{}
		return map[string]interface{}{}, nil
	}

	if !strings.HasPrefix(pathStr, m.root) {
		return nil, fmt.Errorf("mount %s: returned path '%s' has wrong prefix", mnt.Name, pathStr)
	}

	rel, err := filepath.Rel(m.root, pathStr)
	if err != nil {
		return nil, fmt.Errorf("mount %s: relativizing path: %w", mnt.Name, err)
	}

	m.mounts[mnt.Name] = rel
	logger.Debug("mount created", "name", mnt.Name, "source", source, "target", mnt.Target, "path", rel)
	return map[string]interface{}{"path": rel}, nil
}
