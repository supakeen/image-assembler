package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func setupStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreOpenScaffolding(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	for _, sub := range []string{"objects", "stage", "tmp"} {
		p := filepath.Join(dir, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("expected directory %s: %v", sub, err)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", sub)
		}
	}

	for _, f := range []string{"cache.info", "cache.lock", "cache.size"} {
		p := filepath.Join(dir, f)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %s: %v", f, err)
		}
	}
}

func TestContainsEmpty(t *testing.T) {
	s := setupStore(t)
	if s.Contains("nonexistent") {
		t.Error("expected Contains to return false for nonexistent ID")
	}
	if s.Contains("") {
		t.Error("expected Contains to return false for empty ID")
	}
}

func TestNewAndCommit(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	obj, err := s.New(ctx, "test-id")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if obj.Mode() != ModeWrite {
		t.Errorf("expected WRITE mode, got %d", obj.Mode())
	}

	testFile := filepath.Join(obj.Tree(), "hello.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	if err := s.Commit(ctx, obj, "test-id"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if !s.Contains("test-id") {
		t.Error("expected Contains to return true after commit")
	}

	got, err := s.Get(ctx, "test-id")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil object from Get")
	}

	data, err := os.ReadFile(filepath.Join(got.Tree(), "hello.txt"))
	if err != nil {
		t.Fatalf("reading committed file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", data)
	}
}

func TestInitFromBase(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	base, err := s.New(ctx, "base")
	if err != nil {
		t.Fatalf("New base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base.Tree(), "from-base.txt"), []byte("base"), 0644); err != nil {
		t.Fatalf("writing base file: %v", err)
	}
	if err := s.Commit(ctx, base, "base"); err != nil {
		t.Fatalf("Commit base: %v", err)
	}

	baseObj, err := s.Get(ctx, "base")
	if err != nil {
		t.Fatalf("Get base: %v", err)
	}

	derived, err := s.New(ctx, "derived")
	if err != nil {
		t.Fatalf("New derived: %v", err)
	}
	if err := derived.Init(ctx, baseObj); err != nil {
		t.Fatalf("Init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(derived.Tree(), "from-base.txt"))
	if err != nil {
		t.Fatalf("reading inherited file: %v", err)
	}
	if string(data) != "base" {
		t.Errorf("expected 'base', got %q", data)
	}

	if err := os.WriteFile(filepath.Join(derived.Tree(), "new.txt"), []byte("derived"), 0644); err != nil {
		t.Fatalf("writing derived file: %v", err)
	}
	if err := s.Commit(ctx, derived, "derived"); err != nil {
		t.Fatalf("Commit derived: %v", err)
	}

	got, err := s.Get(ctx, "derived")
	if err != nil {
		t.Fatalf("Get derived: %v", err)
	}
	for _, name := range []string{"from-base.txt", "new.txt"} {
		if _, err := os.Stat(filepath.Join(got.Tree(), name)); err != nil {
			t.Errorf("expected file %s in committed derived: %v", name, err)
		}
	}
}

func TestMetadataSetGet(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMetadata(dir, "meta")
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}

	data := map[string]interface{}{"key": "value", "num": float64(42)}
	if err := m.Set("test", data); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := m.Get("test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["key"] != "value" {
		t.Errorf("expected key=value, got %v", got["key"])
	}
}

func TestMetadataWriteEmpty(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMetadata(dir, "meta")
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}

	w, err := m.Write("empty")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(filepath.Join(m.Path(), "empty.json")); !os.IsNotExist(err) {
		t.Error("expected empty.json to not exist")
	}
}

func TestMetadataWriteNonEmpty(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMetadata(dir, "meta")
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}

	w, err := m.Write("data")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	w.file.WriteString(`{"result": true}`)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := m.Get("data")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["result"] != true {
		t.Errorf("expected result=true, got %v", got["result"])
	}
}

func TestMetadataGetMissing(t *testing.T) {
	dir := t.TempDir()
	m, err := NewMetadata(dir, "meta")
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}

	got, err := m.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}

func TestClampMtime(t *testing.T) {
	dir := t.TempDir()

	old := filepath.Join(dir, "old.txt")
	new := filepath.Join(dir, "new.txt")
	os.WriteFile(old, []byte("old"), 0644)
	os.WriteFile(new, []byte("new"), 0644)

	oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(old, oldTime, oldTime)

	createdAt := int(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
	sourceEpoch := int(time.Date(2022, 6, 1, 0, 0, 0, 0, time.UTC).Unix())

	if err := ClampMtime(context.Background(), dir, createdAt, sourceEpoch); err != nil {
		t.Fatalf("ClampMtime: %v", err)
	}

	oldInfo, _ := os.Lstat(old)
	if oldInfo.ModTime().Equal(oldTime) || oldInfo.ModTime().Before(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)) {
		// old file should be unchanged (mtime < createdAt)
	} else {
		t.Errorf("old file mtime should not have changed, got %v", oldInfo.ModTime())
	}

	newInfo, _ := os.Lstat(new)
	expected := time.Unix(int64(sourceEpoch), 0)
	if !newInfo.ModTime().Equal(expected) {
		t.Errorf("new file mtime: expected %v, got %v", expected, newInfo.ModTime())
	}
}

func TestObjectModeTransition(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	obj, err := s.New(ctx, "mode-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if obj.Mode() != ModeWrite {
		t.Errorf("expected WRITE mode initially")
	}

	if err := obj.Finalize(ctx); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if obj.Mode() != ModeRead {
		t.Errorf("expected READ mode after finalize")
	}
}

func TestObjectExport(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	obj, err := s.New(ctx, "export-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	os.WriteFile(filepath.Join(obj.Tree(), "export.txt"), []byte("exported"), 0644)
	if err := s.Commit(ctx, obj, "export-test"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	committed, err := s.Get(ctx, "export-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	exportDir := t.TempDir()
	if err := committed.Export(ctx, exportDir, true); err != nil {
		t.Fatalf("Export: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(exportDir, "export.txt"))
	if err != nil {
		t.Fatalf("reading exported file: %v", err)
	}
	if string(data) != "exported" {
		t.Errorf("expected 'exported', got %q", data)
	}
}

func TestReadOnly(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), dir, true)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	_, err = s.New(context.Background(), "test")
	if err == nil {
		t.Error("expected error for New on read-only store")
	}
}

func TestTempDir(t *testing.T) {
	s := setupStore(t)
	dir, cleanup, err := s.TempDir(context.Background(), "test")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp dir does not exist: %v", err)
	}

	cleanup()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected temp dir to be removed after cleanup")
	}
}

func TestFloatingExportLoad(t *testing.T) {
	dir := t.TempDir()

	s1, err := Open(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx := context.Background()

	obj, err := s1.New(ctx, "float-id")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	os.WriteFile(filepath.Join(obj.Tree(), "test.txt"), []byte("float"), 0644)
	if err := obj.Finalize(ctx); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	exported := s1.ExportFloating()
	s1.Close()

	s2, err := Open(context.Background(), dir, true)
	if err != nil {
		t.Fatalf("Open read-only: %v", err)
	}
	defer s2.Close()

	if err := s2.LoadFloating(exported); err != nil {
		t.Fatalf("LoadFloating: %v", err)
	}

	if !s2.Contains("float-id") {
		t.Error("expected Contains to return true for loaded floating object")
	}
}

func TestDepsolveIntegration(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	obj, err := s.New(ctx, "cached-stage-id")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Commit(ctx, obj, "cached-stage-id"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if !s.Contains("cached-stage-id") {
		t.Error("store should contain committed object")
	}
	if s.Contains("uncached-stage-id") {
		t.Error("store should not contain uncommitted object")
	}
}

func commitObject(t *testing.T, s *Store, id string, dataSize int) {
	t.Helper()
	ctx := context.Background()
	obj, err := s.New(ctx, id)
	if err != nil {
		t.Fatalf("New(%s): %v", id, err)
	}
	if err := os.WriteFile(filepath.Join(obj.Tree(), "data.bin"), make([]byte, dataSize), 0644); err != nil {
		t.Fatalf("writing data for %s: %v", id, err)
	}
	if err := s.Commit(ctx, obj, id); err != nil {
		t.Fatalf("Commit(%s): %v", id, err)
	}
}

func TestCacheSizeTracking(t *testing.T) {
	s := setupStore(t)

	commitObject(t, s, "obj-1", 4096)
	commitObject(t, s, "obj-2", 4096)

	data, err := os.ReadFile(filepath.Join(s.Root(), "cache.size"))
	if err != nil {
		t.Fatalf("reading cache.size: %v", err)
	}
	var size int64
	if err := json.Unmarshal(data, &size); err != nil {
		t.Fatalf("parsing cache.size: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected positive cache size after commits, got %d", size)
	}
}

func TestSetMaximumSize(t *testing.T) {
	s := setupStore(t)

	if err := s.SetMaximumSize(1024 * 1024); err != nil {
		t.Fatalf("SetMaximumSize: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(s.Root(), "cache.info"))
	if err != nil {
		t.Fatalf("reading cache.info: %v", err)
	}
	var info map[string]interface{}
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("parsing cache.info: %v", err)
	}
	v, ok := info["maximum-size"].(float64)
	if !ok {
		t.Fatalf("expected numeric maximum-size, got %T", info["maximum-size"])
	}
	if int64(v) != 1024*1024 {
		t.Errorf("expected maximum-size 1048576, got %v", v)
	}

	if err := s.SetMaximumSize(-1); err != nil {
		t.Fatalf("SetMaximumSize(unlimited): %v", err)
	}
	data, _ = os.ReadFile(filepath.Join(s.Root(), "cache.info"))
	json.Unmarshal(data, &info)
	if info["maximum-size"] != "unlimited" {
		t.Errorf("expected 'unlimited', got %v", info["maximum-size"])
	}
}

func TestSetMaximumSizePersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s1, err := Open(ctx, dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s1.SetMaximumSize(500000)
	s1.Close()

	s2, err := Open(ctx, dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s2.Close()

	if s2.maximumSize != 500000 {
		t.Errorf("expected maximumSize 500000 after reopen, got %d", s2.maximumSize)
	}
}

func TestEvictionLRU(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Phase 1: populate cache with two objects
	s1, err := Open(ctx, dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	commitObject(t, s1, "old-obj", 4096)
	commitObject(t, s1, "new-obj", 4096)
	s1.Close()

	// Set old-obj's atime to 1 hour ago so it's the LRU candidate
	oldLock := filepath.Join(dir, "objects", "old-obj", "object.lock")
	oldTime := time.Now().Add(-1 * time.Hour)
	os.Chtimes(oldLock, oldTime, oldTime)

	// Phase 2: reopen with a size limit and commit another object
	s2, err := Open(ctx, dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s2.Close()

	sizeData, _ := os.ReadFile(filepath.Join(dir, "cache.size"))
	var totalSize int64
	json.Unmarshal(sizeData, &totalSize)

	// Set max just above one object's worth — the third commit will exceed it
	maxSize := (totalSize / 2) + (totalSize / 4)
	if maxSize < 1 {
		maxSize = 1
	}
	s2.SetMaximumSize(maxSize)

	commitObject(t, s2, "trigger-obj", 4096)

	if s2.Contains("old-obj") {
		t.Error("expected old-obj to be evicted (oldest atime)")
	}
	if !s2.Contains("trigger-obj") {
		t.Error("expected trigger-obj to still exist")
	}
}

func TestGetTouchesAtime(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s1, err := Open(ctx, dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	commitObject(t, s1, "atime-test", 4096)
	s1.Close()

	lockPath := filepath.Join(dir, "objects", "atime-test", "object.lock")
	oldTime := time.Now().Add(-1 * time.Hour)
	os.Chtimes(lockPath, oldTime, oldTime)

	var stBefore unix.Stat_t
	unix.Lstat(lockPath, &stBefore)

	s2, err := Open(ctx, dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s2.Close()

	_, err = s2.Get(ctx, "atime-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	var stAfter unix.Stat_t
	unix.Lstat(lockPath, &stAfter)

	if stAfter.Atim.Sec <= stBefore.Atim.Sec {
		t.Errorf("expected atime to be updated after Get, before=%d after=%d",
			stBefore.Atim.Sec, stAfter.Atim.Sec)
	}
}

func TestUnlimitedNoEviction(t *testing.T) {
	s := setupStore(t)
	s.SetMaximumSize(-1)

	commitObject(t, s, "u1", 4096)
	commitObject(t, s, "u2", 4096)
	commitObject(t, s, "u3", 4096)

	for _, id := range []string{"u1", "u2", "u3"} {
		if !s.Contains(id) {
			t.Errorf("expected %s to exist with unlimited cache", id)
		}
	}
}

func TestCommitSizeInObjectInfo(t *testing.T) {
	s := setupStore(t)
	commitObject(t, s, "size-check", 8192)

	infoPath := filepath.Join(s.Root(), "objects", "size-check", "object.info")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		t.Fatalf("reading object.info: %v", err)
	}
	var info map[string]interface{}
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("parsing object.info: %v", err)
	}
	size, ok := info["size"].(float64)
	if !ok {
		t.Fatalf("expected numeric size, got %T", info["size"])
	}
	if int64(size) <= 0 {
		t.Errorf("expected positive size in object.info, got %v", size)
	}
}
