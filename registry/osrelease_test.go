package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func writeOSRelease(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseOSRelease(t *testing.T) {
	path := writeOSRelease(t, `NAME="Fedora Linux"
VERSION_ID=40
ID=fedora
# this is a comment

VARIANT_ID=workstation
`)
	info, err := ParseOSRelease(path)
	if err != nil {
		t.Fatal(err)
	}
	if info["NAME"] != "Fedora Linux" {
		t.Errorf("NAME = %q, want %q", info["NAME"], "Fedora Linux")
	}
	if info["VERSION_ID"] != "40" {
		t.Errorf("VERSION_ID = %q, want %q", info["VERSION_ID"], "40")
	}
	if info["ID"] != "fedora" {
		t.Errorf("ID = %q, want %q", info["ID"], "fedora")
	}
	if info["VARIANT_ID"] != "workstation" {
		t.Errorf("VARIANT_ID = %q, want %q", info["VARIANT_ID"], "workstation")
	}
	if _, ok := info["# this is a comment"]; ok {
		t.Error("comment line should not be parsed")
	}
}

func TestParseOSReleaseQuoting(t *testing.T) {
	path := writeOSRelease(t, `DOUBLE="hello world"
SINGLE='single quoted'
NONE=bare
`)
	info, err := ParseOSRelease(path)
	if err != nil {
		t.Fatal(err)
	}
	if info["DOUBLE"] != "hello world" {
		t.Errorf("DOUBLE = %q, want %q", info["DOUBLE"], "hello world")
	}
	if info["SINGLE"] != "single quoted" {
		t.Errorf("SINGLE = %q, want %q", info["SINGLE"], "single quoted")
	}
	if info["NONE"] != "bare" {
		t.Errorf("NONE = %q, want %q", info["NONE"], "bare")
	}
}

func TestParseOSReleaseFallback(t *testing.T) {
	path := writeOSRelease(t, "ID=centos\n")
	info, err := ParseOSRelease("/nonexistent/os-release", path)
	if err != nil {
		t.Fatal(err)
	}
	if info["ID"] != "centos" {
		t.Errorf("ID = %q, want %q", info["ID"], "centos")
	}
}

func TestParseOSReleaseNoFile(t *testing.T) {
	_, err := ParseOSRelease("/nonexistent/a", "/nonexistent/b")
	if err == nil {
		t.Fatal("expected error when no file exists")
	}
}

func TestDescribeOS(t *testing.T) {
	path := writeOSRelease(t, "ID=fedora\nVERSION_ID=40\n")
	got, err := DescribeOS(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fedora40" {
		t.Errorf("DescribeOS = %q, want %q", got, "fedora40")
	}
}

func TestDescribeOSDotVersion(t *testing.T) {
	path := writeOSRelease(t, "ID=rhel\nVERSION_ID=8.9\n")
	got, err := DescribeOS(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "rhel89" {
		t.Errorf("DescribeOS = %q, want %q", got, "rhel89")
	}
}

func TestDescribeOSMissingID(t *testing.T) {
	path := writeOSRelease(t, "VERSION_ID=42\n")
	got, err := DescribeOS(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "linux42" {
		t.Errorf("DescribeOS = %q, want %q", got, "linux42")
	}
}
