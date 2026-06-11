package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSizeBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"0", 0},
		{"1024", 1024},
		{"  512  ", 512},
	}
	for _, tt := range tests {
		got, err := ParseSize(tt.input)
		if err != nil {
			t.Errorf("ParseSize(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseSize(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseSizeSI(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1kB", 1000},
		{"10kB", 10000},
		{"1MB", 1000 * 1000},
		{"1GB", 1000 * 1000 * 1000},
		{"1TB", 1000 * 1000 * 1000 * 1000},
	}
	for _, tt := range tests {
		got, err := ParseSize(tt.input)
		if err != nil {
			t.Errorf("ParseSize(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseSize(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseSizeBinary(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1KiB", 1024},
		{"1MiB", 1024 * 1024},
		{"1GiB", 1024 * 1024 * 1024},
		{"1TiB", 1024 * 1024 * 1024 * 1024},
	}
	for _, tt := range tests {
		got, err := ParseSize(tt.input)
		if err != nil {
			t.Errorf("ParseSize(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseSize(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseSizeUnlimited(t *testing.T) {
	got, err := ParseSize("unlimited")
	if err != nil {
		t.Fatalf("ParseSize(unlimited) error: %v", err)
	}
	if got != -1 {
		t.Errorf("ParseSize(unlimited) = %d, want -1", got)
	}
}

func TestParseSizeInvalid(t *testing.T) {
	invalids := []string{"", "abc", "1PB", "1.5GB", "-100"}
	for _, s := range invalids {
		_, err := ParseSize(s)
		if err == nil {
			t.Errorf("ParseSize(%q) should return error", s)
		}
	}
}

func TestCalculateSpace(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), make([]byte, 4096), 0644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), make([]byte, 8192), 0644); err != nil {
		t.Fatal(err)
	}

	size, err := calculateSpace(dir)
	if err != nil {
		t.Fatalf("calculateSpace: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}
	// At minimum the data files should account for some blocks
	if size < 4096+8192 {
		t.Errorf("size %d seems too small for 12KiB of file data", size)
	}
}

func TestCalculateSpaceEmpty(t *testing.T) {
	dir := t.TempDir()
	size, err := calculateSpace(dir)
	if err != nil {
		t.Fatalf("calculateSpace: %v", err)
	}
	if size < 0 {
		t.Errorf("expected non-negative size for empty dir, got %d", size)
	}
}
