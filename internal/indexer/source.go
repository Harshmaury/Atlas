// @atlas-project: atlas
// @atlas-path: internal/indexer/source.go
// Package indexer indexes source files and architecture documents
// into the Atlas store for search and context generation.
package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harshmaury/Atlas/internal/discovery"
	"github.com/Harshmaury/Atlas/internal/store"
)

// ── CONSTANTS ────────────────────────────────────────────────────────────────

const (
	maxFileSizeBytes  = 2 * 1024 * 1024 // 2MB — skip large generated files
	maxFilesPerProject = 5000
)

// ── SOURCE INDEXER ───────────────────────────────────────────────────────────

// SourceIndexer indexes source files for a project.
type SourceIndexer struct {
	store store.Storer
}

// NewSourceIndexer creates a SourceIndexer.
func NewSourceIndexer(s store.Storer) *SourceIndexer {
	return &SourceIndexer{store: s}
}

// IndexProject walks a project directory and indexes all source files.
// Replaces any existing index entries for the project.
func (idx *SourceIndexer) IndexProject(p *store.Project) (int, error) {
	if p.Path == "" {
		return 0, nil
	}

	if _, err := os.Stat(p.Path); err != nil {
		return 0, fmt.Errorf("project path not found: %s", p.Path)
	}

	count := 0
	err := filepath.WalkDir(p.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if isSkippedDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if count >= maxFilesPerProject {
			return filepath.SkipDir
		}

		ext := filepath.Ext(path)
		lang := discovery.LanguageForExtension(ext)
		if lang == "" {
			return nil // skip files with unknown extensions
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > maxFileSizeBytes {
			return nil
		}

		if err := idx.store.UpsertFile(&store.File{
			ProjectID: p.ID,
			Path:      path,
			Language:  lang,
			SizeBytes: info.Size(),
		}); err != nil {
			return fmt.Errorf("index file %s: %w", path, err)
		}
		count++
		return nil
	})

	return count, err
}

// ── HELPERS ──────────────────────────────────────────────────────────────────

func isSkippedDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".cache", "__pycache__",
		"dist", "build", "target", ".nexus":
		return true
	}
	return strings.HasPrefix(name, ".")
}
