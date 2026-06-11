// Package store implements a content-addressed object store for build
// artifacts, matching Python osbuild's objectstore.py. Objects live on disk
// under a root directory with this layout:
//
//	<root>/objects/<id>/data/   — committed, immutable cache entries
//	<root>/stage/uuid-<uuid>/   — writable workspace for in-flight builds
//	<root>/tmp/                 — ephemeral temp directories
//	<root>/cache.info           — store metadata (version, maximum-size)
//	<root>/cache.size           — aggregate size of all committed objects
//	<root>/cache.lock           — flock for concurrent access
//
// Objects are created in stage/, built up by pipeline stages, then atomically
// committed to objects/ via copy-on-write (cp --reflink=auto). Committed
// objects are keyed by their content hash (stage/pipeline ID) and are
// immutable. An LRU eviction policy removes least-recently-used objects
// when the cache exceeds --cache-max-size.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	ilog "github.com/supakeen/image-assembler/internal/log"
	"golang.org/x/sys/unix"
)

// Store manages the on-disk object cache. It holds a shared flock on
// cache.lock for the lifetime of the process, allowing concurrent readers
// while serializing eviction via exclusive locks on cache.size.
type Store struct {
	root        string
	tmp         string
	objs        map[string]*Object // in-flight and floating objects, keyed by ID
	mu          sync.Mutex
	readOnly    bool
	lockFile    *os.File
	hostTree    *HostTree
	maximumSize int64 // -1 = unlimited, 0 = no limit (default), >0 = byte limit
}

// Open initializes the store directory structure and acquires a shared lock.
// If the directory doesn't exist it is created along with all subdirectories.
// Any persisted maximum-size setting in cache.info is restored.
func Open(ctx context.Context, storePath string, readOnly bool) (*Store, error) {
	root, err := filepath.Abs(storePath)
	if err != nil {
		return nil, fmt.Errorf("resolving store path: %w", err)
	}

	for _, dir := range []string{
		root,
		filepath.Join(root, "objects"),
		filepath.Join(root, "stage"),
		filepath.Join(root, "tmp"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating store directory %s: %w", dir, err)
		}
	}

	cacheInfo := filepath.Join(root, "cache.info")
	var maximumSize int64
	if data, err := os.ReadFile(cacheInfo); err == nil {
		var info map[string]interface{}
		if json.Unmarshal(data, &info) == nil {
			switch v := info["maximum-size"].(type) {
			case float64:
				maximumSize = int64(v)
			case string:
				if v == "unlimited" {
					maximumSize = -1
				}
			}
		}
	} else if os.IsNotExist(err) {
		data, _ := json.Marshal(map[string]interface{}{"version": 1})
		if err := os.WriteFile(cacheInfo, data, 0644); err != nil {
			return nil, fmt.Errorf("writing cache.info: %w", err)
		}
	}

	cacheSize := filepath.Join(root, "cache.size")
	if _, err := os.Stat(cacheSize); os.IsNotExist(err) {
		if err := os.WriteFile(cacheSize, []byte("0"), 0644); err != nil {
			return nil, fmt.Errorf("writing cache.size: %w", err)
		}
	}

	lockPath := filepath.Join(root, "cache.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening cache.lock: %w", err)
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_SH); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("acquiring cache lock: %w", err)
	}

	logger := ilog.FromContext(ctx)
	logger.Info("store opened", "path", root, "read_only", readOnly)

	return &Store{
		root:        root,
		tmp:         filepath.Join(root, "tmp"),
		objs:        make(map[string]*Object),
		readOnly:    readOnly,
		lockFile:    lockFile,
		maximumSize: maximumSize,
	}, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) Contains(id string) bool {
	if id == "" {
		return false
	}

	s.mu.Lock()
	if _, ok := s.objs[id]; ok {
		s.mu.Unlock()
		return true
	}
	s.mu.Unlock()

	lockPath := filepath.Join(s.root, "objects", id, "object.lock")
	_, err := os.Stat(lockPath)
	return err == nil
}

// Get retrieves a committed or floating object by ID. When loading from disk,
// it touches the object.lock atime to mark it as recently used for LRU
// eviction. Returns nil (no error) if the object doesn't exist.
func (s *Store) Get(ctx context.Context, id string) (*Object, error) {
	logger := ilog.FromContext(ctx)

	s.mu.Lock()
	if obj, ok := s.objs[id]; ok {
		s.mu.Unlock()
		logger.Debug("store get", "id", id, "hit", true, "source", "memory")
		return obj, nil
	}
	s.mu.Unlock()

	objDir := filepath.Join(s.root, "objects", id)
	dataPath := filepath.Join(objDir, "data")
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		logger.Debug("store get", "id", id, "hit", false)
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking object %s: %w", id, err)
	}

	lockPath := filepath.Join(objDir, "object.lock")
	unix.UtimesNanoAt(unix.AT_FDCWD, lockPath, []unix.Timespec{
		{Sec: 0, Nsec: unix.UTIME_NOW},
		{Sec: 0, Nsec: unix.UTIME_OMIT},
	}, 0)

	meta, err := NewMetadata(dataPath, "meta")
	if err != nil {
		return nil, fmt.Errorf("loading object metadata: %w", err)
	}

	logger.Debug("store get", "id", id, "hit", true, "source", "disk")

	return &Object{
		id:   id,
		mode: ModeRead,
		path: dataPath,
		meta: meta,
	}, nil
}

// New creates a writable staging object. The caller populates its Tree()
// directory, then passes it to Commit to persist it in the cache.
func (s *Store) New(ctx context.Context, id string) (*Object, error) {
	if s.readOnly {
		return nil, fmt.Errorf("store is read-only")
	}

	uuid := newUUID()
	stageDir := filepath.Join(s.root, "stage", "uuid-"+uuid)
	dataDir := filepath.Join(stageDir, "data")
	treeDir := filepath.Join(dataDir, "tree")

	if err := os.MkdirAll(treeDir, 0755); err != nil {
		return nil, fmt.Errorf("creating stage directory: %w", err)
	}

	lockPath := filepath.Join(stageDir, "object.lock")
	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		os.RemoveAll(stageDir)
		return nil, fmt.Errorf("creating object.lock: %w", err)
	}

	meta, err := NewMetadata(dataDir, "meta")
	if err != nil {
		os.RemoveAll(stageDir)
		return nil, fmt.Errorf("creating metadata: %w", err)
	}

	if err := meta.Set("info", map[string]interface{}{
		"created": int(time.Now().Unix()),
	}); err != nil {
		os.RemoveAll(stageDir)
		return nil, fmt.Errorf("writing info metadata: %w", err)
	}

	obj := &Object{
		id:        id,
		mode:      ModeWrite,
		path:      dataDir,
		stagePath: stageDir,
		meta:      meta,
	}

	s.mu.Lock()
	s.objs[id] = obj
	s.mu.Unlock()

	ilog.FromContext(ctx).Info("object created", "id", id, "stage_dir", stageDir)

	return obj, nil
}

// Commit copies a staging object to the permanent cache under objectID.
// It uses cp --reflink=auto for copy-on-write on supporting filesystems,
// calculates the committed size for cache tracking, and triggers LRU
// eviction if the cache exceeds its maximum size. Idempotent — returns
// nil if objectID is already committed.
func (s *Store) Commit(ctx context.Context, obj *Object, objectID string) error {
	logger := ilog.FromContext(ctx)

	if s.readOnly {
		return fmt.Errorf("store is read-only")
	}

	obj.ClampMtime(ctx)

	targetDir := filepath.Join(s.root, "objects", objectID)
	if _, err := os.Stat(targetDir); err == nil {
		logger.Debug("object already committed", "id", objectID)
		return nil
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating object directory: %w", err)
	}

	lockPath := filepath.Join(targetDir, "object.lock")
	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		os.RemoveAll(targetDir)
		return fmt.Errorf("creating object.lock: %w", err)
	}

	cmd := exec.CommandContext(ctx, "cp", "--reflink=auto", "-a",
		obj.Path()+"/.", filepath.Join(targetDir, "data"))
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(targetDir)
		return fmt.Errorf("committing object: %s: %w", out, err)
	}

	committedData := filepath.Join(targetDir, "data")
	objSize, _ := calculateSpace(committedData)

	info := map[string]interface{}{
		"creation-boot-id": "",
		"size":             objSize,
	}
	infoJSON, _ := json.Marshal(info)
	if err := os.WriteFile(filepath.Join(targetDir, "object.info"), infoJSON, 0644); err != nil {
		os.RemoveAll(targetDir)
		return fmt.Errorf("writing object.info: %w", err)
	}

	if obj.stagePath != "" {
		os.RemoveAll(obj.stagePath)
		obj.stagePath = ""
	}

	obj.path = committedData
	obj.mode = ModeRead

	if !s.updateCacheSize(ctx, objSize) {
		s.removeLRU(ctx, objSize)
		if !s.updateCacheSize(ctx, objSize) {
			logger.Warn("cache over limit after eviction", "size", objSize)
		}
	}

	logger.Info("object committed", "id", objectID, "path", targetDir, "size", objSize)

	return nil
}

// SetMaximumSize sets the cache size limit and persists it to cache.info.
// Use -1 for unlimited, 0 to clear the limit, or a positive byte count.
func (s *Store) SetMaximumSize(size int64) error {
	s.maximumSize = size

	cacheInfo := filepath.Join(s.root, "cache.info")
	data, err := os.ReadFile(cacheInfo)
	if err != nil {
		return fmt.Errorf("reading cache.info: %w", err)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(data, &info); err != nil {
		info = map[string]interface{}{"version": 1}
	}

	if size < 0 {
		info["maximum-size"] = "unlimited"
	} else if size == 0 {
		delete(info, "maximum-size")
	} else {
		info["maximum-size"] = size
	}

	out, _ := json.Marshal(info)
	return os.WriteFile(cacheInfo, out, 0644)
}

func (s *Store) updateCacheSize(ctx context.Context, diff int64) bool {
	logger := ilog.FromContext(ctx)
	sizePath := filepath.Join(s.root, "cache.size")

	f, err := os.OpenFile(sizePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		logger.Warn("failed to open cache.size", "error", err)
		return true
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		logger.Warn("failed to lock cache.size", "error", err)
		return true
	}

	data, _ := os.ReadFile(sizePath)
	var size int64
	json.Unmarshal(data, &size)

	newSize := size + diff
	if newSize < 0 {
		newSize = 0
	}

	if diff > 0 && s.maximumSize > 0 && newSize > s.maximumSize {
		return false
	}

	out, _ := json.Marshal(newSize)
	os.WriteFile(sizePath, out, 0644)
	return true
}

type objectInfo struct {
	name     string
	lastUsed int64
}

func (s *Store) removeLRU(ctx context.Context, requiredSize int64) bool {
	logger := ilog.FromContext(ctx)
	objsDir := filepath.Join(s.root, "objects")

	entries, err := os.ReadDir(objsDir)
	if err != nil {
		return false
	}

	var objs []objectInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		lockPath := filepath.Join(objsDir, e.Name(), "object.lock")
		var st unix.Stat_t
		if err := unix.Lstat(lockPath, &st); err != nil {
			continue
		}
		objs = append(objs, objectInfo{
			name:     e.Name(),
			lastUsed: st.Atim.Sec,
		})
	}

	sort.Slice(objs, func(i, j int) bool {
		return objs[i].lastUsed < objs[j].lastUsed
	})

	tryToFree := requiredSize * 2
	var freedSoFar int64

	for _, obj := range objs {
		s.mu.Lock()
		_, inUse := s.objs[obj.name]
		s.mu.Unlock()
		if inUse {
			continue
		}

		lockPath := filepath.Join(objsDir, obj.name, "object.lock")
		var st unix.Stat_t
		if err := unix.Lstat(lockPath, &st); err != nil {
			continue
		}
		if st.Atim.Sec != obj.lastUsed {
			continue
		}

		objDir := filepath.Join(objsDir, obj.name)
		dataDir := filepath.Join(objDir, "data")
		size, _ := calculateSpace(dataDir)
		if size == 0 {
			infoPath := filepath.Join(objDir, "object.info")
			if data, err := os.ReadFile(infoPath); err == nil {
				var info map[string]interface{}
				if json.Unmarshal(data, &info) == nil {
					if v, ok := info["size"].(float64); ok {
						size = int64(v)
					}
				}
			}
		}

		logger.Info("evicting object", "id", obj.name, "size", size)
		os.RemoveAll(objDir)
		s.updateCacheSize(ctx, -size)
		freedSoFar += size

		if freedSoFar >= tryToFree {
			break
		}
	}

	return freedSoFar >= requiredSize
}

func (s *Store) TempDir(_ context.Context, prefix string) (string, func(), error) {
	dir, err := os.MkdirTemp(s.tmp, prefix)
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir: %w", err)
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

func (s *Store) GetHostTree(ctx context.Context) (*HostTree, error) {
	if s.hostTree != nil {
		return s.hostTree, nil
	}
	ht, err := NewHostTree(ctx, s)
	if err != nil {
		return nil, err
	}
	s.hostTree = ht
	return ht, nil
}

// ExportFloating serializes in-memory read-only objects for transfer to
// another process (e.g. a VM). The receiving process calls LoadFloating
// to reconstruct them.
func (s *Store) ExportFloating() []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []map[string]interface{}
	for _, obj := range s.objs {
		if obj.mode == ModeRead {
			result = append(result, obj.ToDict(s.root))
		}
	}
	return result
}

func (s *Store) LoadFloating(objs []map[string]interface{}) error {
	if !s.readOnly {
		return fmt.Errorf("store must be read-only to use LoadFloating")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.objs = make(map[string]*Object)
	for _, data := range objs {
		obj, err := ObjectFromDict(data, s.root)
		if err != nil {
			return err
		}
		s.objs[obj.id] = obj
	}
	return nil
}

func (s *Store) Close() error {
	if s.hostTree != nil {
		s.hostTree.Close()
		s.hostTree = nil
	}

	s.mu.Lock()
	for _, obj := range s.objs {
		obj.Close()
	}
	s.objs = make(map[string]*Object)
	s.mu.Unlock()

	if s.lockFile != nil {
		syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
		s.lockFile.Close()
		s.lockFile = nil
	}
	return nil
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}
