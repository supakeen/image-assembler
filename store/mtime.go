package store

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// ClampMtime enforces reproducible builds by resetting the modification time
// of any file newer than createdAt to sourceEpoch. This ensures that
// identical inputs produce identical outputs regardless of wall-clock time
// during the build.
func ClampMtime(ctx context.Context, treePath string, createdAt int, sourceEpoch int) error {
	startTime := time.Unix(int64(createdAt), 0)
	target := unix.NsecToTimespec(time.Unix(int64(sourceEpoch), 0).UnixNano())
	times := [2]unix.Timespec{target, target}

	clamp := func(path string) error {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.ModTime().Before(startTime) {
			return nil
		}
		err = unix.UtimesNanoAt(unix.AT_FDCWD, path, times[:], unix.AT_SYMLINK_NOFOLLOW)
		if err != nil && errors.Is(err, unix.EPERM) {
			return nil
		}
		return err
	}

	if err := clamp(treePath); err != nil {
		return err
	}

	return filepath.WalkDir(treePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return clamp(path)
	})
}
