package registry

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var DefaultOSReleasePaths = []string{"/etc/os-release", "/usr/lib/os-release"}

func ParseOSRelease(paths ...string) (map[string]string, error) {
	var f *os.File
	var err error
	for _, p := range paths {
		f, err = os.Open(p)
		if err == nil {
			break
		}
	}
	if f == nil {
		return nil, fmt.Errorf("no os-release file found")
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = unquote(val)
		result[key] = val
	}
	return result, scanner.Err()
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return s
}

// DescribeOS returns a compact OS identifier (e.g. "fedora40") suitable for
// matching against runner names. It concatenates ID and VERSION_ID from
// os-release, stripping dots from the version.
func DescribeOS(paths ...string) (string, error) {
	if len(paths) == 0 {
		paths = DefaultOSReleasePaths
	}
	info, err := ParseOSRelease(paths...)
	if err != nil {
		return "", err
	}
	id := info["ID"]
	if id == "" {
		id = "linux"
	}
	versionID := info["VERSION_ID"]
	versionID = strings.ReplaceAll(versionID, ".", "")
	return id + versionID, nil
}
