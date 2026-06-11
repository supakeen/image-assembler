package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/supakeen/image-assembler/ipc"
	"github.com/supakeen/image-assembler/store"
)

func setupStoreServer(t *testing.T) (*StoreServer, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv := NewStoreServer(st)
	srv.SocketAddress = filepath.Join(t.TempDir(), "store.sock")

	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Cleanup() })

	return srv, st
}

func storeServerCall(t *testing.T, addr string, msg map[string]interface{}) map[string]interface{} {
	t.Helper()
	client, err := ipc.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Send(msg, nil); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var reply map[string]interface{}
	var recvErr error

	go func() {
		reply, _, recvErr = client.Recv()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for reply")
	}

	if recvErr != nil {
		t.Fatal(recvErr)
	}
	return reply
}

func commitTestObject(t *testing.T, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	obj, err := st.New(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(obj.Tree(), "data.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := obj.Finalize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.Commit(ctx, obj, id); err != nil {
		t.Fatal(err)
	}
}

func TestStoreServerReadTree(t *testing.T) {
	srv, st := setupStoreServer(t)

	objectID := "abc123def456"
	commitTestObject(t, st, objectID)

	reply := storeServerCall(t, srv.SocketAddress, map[string]interface{}{
		"method":    "read-tree",
		"object-id": objectID,
	})

	path, ok := reply["path"].(string)
	if !ok || path == "" {
		t.Fatalf("expected non-empty path, got %v", reply["path"])
	}

	if _, err := os.Stat(filepath.Join(path, "data.txt")); err != nil {
		t.Errorf("expected data.txt in tree path: %v", err)
	}
}

func TestStoreServerReadTreeMissing(t *testing.T) {
	srv, _ := setupStoreServer(t)

	reply := storeServerCall(t, srv.SocketAddress, map[string]interface{}{
		"method":    "read-tree",
		"object-id": "nonexistent",
	})

	if reply["path"] != nil {
		t.Errorf("expected nil path for missing object, got %v", reply["path"])
	}
}

func TestStoreServerMkdtemp(t *testing.T) {
	srv, _ := setupStoreServer(t)

	reply := storeServerCall(t, srv.SocketAddress, map[string]interface{}{
		"method": "mkdtemp",
		"prefix": "test-",
		"suffix": "-dir",
	})

	path, ok := reply["path"].(string)
	if !ok || path == "" {
		t.Fatalf("expected path, got %v", reply)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("path does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestStoreServerSource(t *testing.T) {
	srv, st := setupStoreServer(t)

	reply := storeServerCall(t, srv.SocketAddress, map[string]interface{}{
		"method": "source",
		"name":   "org.osbuild.curl",
	})

	path, ok := reply["path"].(string)
	if !ok {
		t.Fatalf("expected path string, got %v", reply)
	}

	expected := filepath.Join(st.Root(), "sources", "org.osbuild.curl")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}
