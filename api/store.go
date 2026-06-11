package api

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/supakeen/image-assembler/ipc"
	"github.com/supakeen/image-assembler/store"
)

// StoreServer gives stages access to the object store from inside the
// sandbox. It handles read-tree (bind-mount an object's tree into the
// sandbox), mkdtemp (create temp directories), and source path lookups.
// All bind mounts are cleaned up via CleanupMounts after the stage exits.
type StoreServer struct {
	*Server
	store   *store.Store
	tmpRoot string
	mounts  []string
}

func NewStoreServer(st *store.Store) *StoreServer {
	tmpRoot, _, _ := st.TempDir(context.Background(), "store-server-")
	s := &StoreServer{
		store:   st,
		tmpRoot: tmpRoot,
	}
	s.Server = NewServer("store", s.handle)
	return s
}

func (s *StoreServer) handle(msg map[string]interface{}, fds *ipc.FDSet, sock *ipc.Socket) {
	if fds != nil {
		fds.Close()
	}

	method, _ := msg["method"].(string)
	switch method {
	case "read-tree":
		s.readTree(msg, sock)
	case "read-tree-at":
		s.readTreeAt(msg, sock)
	case "mkdtemp":
		s.mkdtemp(msg, sock)
	case "source":
		s.source(msg, sock)
	}
}

func (s *StoreServer) readTree(msg map[string]interface{}, sock *ipc.Socket) {
	objectID, _ := msg["object-id"].(string)
	obj, err := s.store.Get(context.Background(), objectID)
	if err != nil || obj == nil {
		sock.Send(map[string]interface{}{"path": nil}, nil)
		return
	}
	sock.Send(map[string]interface{}{"path": obj.Tree()}, nil)
}

func (s *StoreServer) readTreeAt(msg map[string]interface{}, sock *ipc.Socket) {
	objectID, _ := msg["object-id"].(string)
	target, _ := msg["target"].(string)
	subtree, _ := msg["subtree"].(string)

	obj, err := s.store.Get(context.Background(), objectID)
	if err != nil || obj == nil {
		sock.Send(map[string]interface{}{"path": nil}, nil)
		return
	}

	source := filepath.Join(obj.Tree(), subtree)

	if err := exec.Command("mount", "--bind", source, target).Run(); err != nil {
		sock.Send(map[string]interface{}{"error": fmt.Sprintf("mount bind: %v", err)}, nil)
		return
	}
	s.mounts = append(s.mounts, target)

	sock.Send(map[string]interface{}{"path": target}, nil)
}

func (s *StoreServer) mkdtemp(msg map[string]interface{}, sock *ipc.Socket) {
	prefix, _ := msg["prefix"].(string)
	suffix, _ := msg["suffix"].(string)

	pattern := prefix + "*" + suffix
	path, err := os.MkdirTemp(s.tmpRoot, pattern)
	if err != nil {
		sock.Send(map[string]interface{}{"error": err.Error()}, nil)
		return
	}

	sock.Send(map[string]interface{}{"path": path}, nil)
}

func (s *StoreServer) source(msg map[string]interface{}, sock *ipc.Socket) {
	name, _ := msg["name"].(string)
	path := filepath.Join(s.store.Root(), "sources", name)
	sock.Send(map[string]interface{}{"path": path}, nil)
}

func (s *StoreServer) CleanupMounts() {
	for i := len(s.mounts) - 1; i >= 0; i-- {
		exec.Command("umount", s.mounts[i]).Run()
	}
	s.mounts = nil
	if s.tmpRoot != "" {
		os.RemoveAll(s.tmpRoot)
	}
}
