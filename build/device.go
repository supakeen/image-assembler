package build

import (
	"context"
	"fmt"
	"path/filepath"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/service"
)

// DeviceManager opens block devices for a stage by calling their Python
// service processes. It tracks opened devices and their returned paths so
// mounts and other devices can reference them by name.
type DeviceManager struct {
	services *service.Manager
	DevPath  string
	TreePath string
	devices  map[string]map[string]interface{}
}

func NewDeviceManager(mgr *service.Manager, devPath, treePath string) *DeviceManager {
	return &DeviceManager{
		services: mgr,
		DevPath:  devPath,
		TreePath: treePath,
		devices:  make(map[string]map[string]interface{}),
	}
}

func (m *DeviceManager) RelPath(dev *manifest.Device) string {
	if dev == nil {
		return ""
	}
	res, ok := m.devices[dev.Name]
	if !ok {
		return ""
	}
	path, _ := res["path"].(string)
	return path
}

func (m *DeviceManager) AbsPath(dev *manifest.Device) string {
	rel := m.RelPath(dev)
	if rel == "" {
		return ""
	}
	return filepath.Join(m.DevPath, rel)
}

func (m *DeviceManager) Open(ctx context.Context, dev *manifest.Device) (map[string]interface{}, error) {
	parent := m.RelPath(dev.Parent)

	args := map[string]interface{}{
		"dev":     m.DevPath,
		"tree":    m.TreePath,
		"parent":  parent,
		"options": dev.Options,
	}

	client, err := m.services.Start(ctx, "device/"+dev.Name, dev.ExecPath, nil)
	if err != nil {
		return nil, fmt.Errorf("starting device service %s: %w", dev.Name, err)
	}

	reply, err := client.Call(ctx, "open", args)
	if err != nil {
		return nil, fmt.Errorf("calling device open %s: %w", dev.Name, err)
	}

	res, ok := reply.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("device %s: unexpected reply type %T", dev.Name, reply)
	}

	m.devices[dev.Name] = res
	path, _ := res["path"].(string)
	ilog.FromContext(ctx).Debug("device opened", "name", dev.Name, "type", dev.Type, "path", path)
	return res, nil
}
