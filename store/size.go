package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

var sizePatterns = []struct {
	re    *regexp.Regexp
	base  int64
	power int
}{
	{regexp.MustCompile(`^\s*(\d+)\s*kB$`), 1000, 1},
	{regexp.MustCompile(`^\s*(\d+)\s*KiB$`), 1024, 1},
	{regexp.MustCompile(`^\s*(\d+)\s*MB$`), 1000, 2},
	{regexp.MustCompile(`^\s*(\d+)\s*MiB$`), 1024, 2},
	{regexp.MustCompile(`^\s*(\d+)\s*GB$`), 1000, 3},
	{regexp.MustCompile(`^\s*(\d+)\s*GiB$`), 1024, 3},
	{regexp.MustCompile(`^\s*(\d+)\s*TB$`), 1000, 4},
	{regexp.MustCompile(`^\s*(\d+)\s*TiB$`), 1024, 4},
	{regexp.MustCompile(`^\s*(\d+)\s*$`), 1, 1},
}

// ParseSize parses a human-readable size string into bytes. Accepted formats
// match Python osbuild's parse_size(): raw bytes ("1024"), SI units (kB, MB,
// GB, TB), binary units (KiB, MiB, GiB, TiB), or "unlimited" (returns -1).
func ParseSize(s string) (int64, error) {
	if strings.TrimSpace(s) == "unlimited" {
		return -1, nil
	}

	for _, p := range sizePatterns {
		m := p.re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size value: %q", s)
		}
		result := n
		for i := 0; i < p.power; i++ {
			result *= p.base
		}
		return result, nil
	}

	return 0, fmt.Errorf("invalid size value: %q", s)
}

func calculateSpace(root string) (int64, error) {
	var total int64

	var st syscall.Stat_t
	if err := syscall.Lstat(root, &st); err != nil {
		return 0, fmt.Errorf("stat %s: %w", root, err)
	}
	total += st.Blocks * 512

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		var s syscall.Stat_t
		if err := syscall.Lstat(path, &s); err != nil {
			return err
		}
		total += s.Blocks * 512
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walking %s: %w", root, err)
	}

	return total, nil
}
