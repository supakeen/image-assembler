package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Metadata provides keyed JSON storage for build results. Each stage writes
// its output metadata under a key matching its ID, and the build system
// later reads it back for the JSON result output.
type Metadata struct {
	basePath string
	folder   string
}

func NewMetadata(basePath, folder string) (*Metadata, error) {
	m := &Metadata{basePath: basePath, folder: folder}
	if err := os.MkdirAll(m.Path(), 0755); err != nil {
		return nil, fmt.Errorf("creating metadata dir: %w", err)
	}
	return m, nil
}

func (m *Metadata) Path() string {
	if m.folder == "" {
		return m.basePath
	}
	return filepath.Join(m.basePath, m.folder)
}

func (m *Metadata) pathForKey(key string) string {
	return filepath.Join(m.Path(), key+".json")
}

func (m *Metadata) Set(key string, data interface{}) error {
	if data == nil {
		return nil
	}
	w, err := m.Write(key)
	if err != nil {
		return err
	}
	defer w.Close()

	enc := json.NewEncoder(w.file)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func (m *Metadata) Get(key string) (map[string]interface{}, error) {
	f, err := os.Open(m.pathForKey(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(f).Decode(&result); err != nil {
		return nil, fmt.Errorf("reading metadata %s: %w", key, err)
	}
	return result, nil
}

func (m *Metadata) Write(key string) (*MetadataWriter, error) {
	f, err := os.CreateTemp(m.Path(), "."+key+"*.tmp.json")
	if err != nil {
		return nil, fmt.Errorf("creating metadata temp file: %w", err)
	}
	return &MetadataWriter{
		file:    f,
		Name:    f.Name(),
		key:     key,
		metaDir: m.Path(),
	}, nil
}

// MetadataWriter wraps a temp file for atomic metadata writes. The stage
// process writes its output to Name (the temp file path), then Close
// hard-links the result to the final key.json path. If the stage produces
// no output (empty file), Close removes the temp file without linking,
// so absent metadata cleanly indicates "no output".
type MetadataWriter struct {
	file    *os.File
	Name    string
	key     string
	metaDir string
}

func (w *MetadataWriter) Close() error {
	if err := w.file.Sync(); err != nil {
		w.file.Close()
		os.Remove(w.Name)
		return err
	}

	info, err := w.file.Stat()
	if err != nil {
		w.file.Close()
		os.Remove(w.Name)
		return err
	}

	w.file.Close()

	if info.Size() == 0 {
		os.Remove(w.Name)
		return nil
	}

	dest := filepath.Join(w.metaDir, w.key+".json")
	if err := os.Link(w.Name, dest); err != nil {
		os.Remove(w.Name)
		return fmt.Errorf("linking metadata %s: %w", w.key, err)
	}

	os.Remove(w.Name)
	return nil
}
