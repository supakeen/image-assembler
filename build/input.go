package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/supakeen/image-assembler/api"
	ilog "github.com/supakeen/image-assembler/internal/log"
	"github.com/supakeen/image-assembler/manifest"
	"github.com/supakeen/image-assembler/service"
)

// InputManager maps inputs for a stage by starting their Python service
// processes and calling "map". It provides each input with the store API
// socket address so inputs with pipeline-origin refs can read other
// pipelines' trees from the store.
type InputManager struct {
	services *service.Manager
	storeAPI *api.StoreServer
	root     string
	inputs   map[string]interface{}
}

func NewInputManager(mgr *service.Manager, storeAPI *api.StoreServer, root string) *InputManager {
	return &InputManager{
		services: mgr,
		storeAPI: storeAPI,
		root:     root,
		inputs:   make(map[string]interface{}),
	}
}

func (m *InputManager) Map(ctx context.Context, ip *manifest.Input) (map[string]interface{}, error) {
	target := filepath.Join(m.root, ip.Name)
	os.MkdirAll(target, 0o755)

	args := map[string]interface{}{
		"origin":  ip.Origin,
		"refs":    ip.Refs,
		"target":  target,
		"options": ip.Options,
		"api": map[string]interface{}{
			"store": m.storeAPI.SocketAddress,
		},
	}

	client, err := m.services.Start(ctx, "input/"+ip.Name, ip.ExecPath, nil)
	if err != nil {
		return nil, fmt.Errorf("starting input service %s: %w", ip.Name, err)
	}

	reply, err := client.Call(ctx, "map", args)
	if err != nil {
		return nil, fmt.Errorf("calling input map %s: %w", ip.Name, err)
	}

	replyMap, ok := reply.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("input %s: unexpected reply type %T", ip.Name, reply)
	}

	if path, ok := replyMap["path"].(string); ok {
		if !strings.HasPrefix(path, m.root) {
			return nil, fmt.Errorf("input %s: returned path %s has wrong prefix", ip.Name, path)
		}
		rel, err := filepath.Rel(m.root, path)
		if err != nil {
			return nil, fmt.Errorf("input %s: relativizing path: %w", ip.Name, err)
		}
		replyMap["path"] = rel
	}

	m.inputs[ip.Name] = replyMap
	ilog.FromContext(ctx).Debug("input mapped", "name", ip.Name, "origin", ip.Origin)
	return replyMap, nil
}
