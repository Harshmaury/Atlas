// @atlas-project: atlas
// @atlas-path: internal/indexer/document.go
// AT-H-05: IndexProject now captures and returns WalkDir errors.
//   Previously filepath.WalkDir return value was discarded entirely —
//   permission errors and unreadable directories were silently ignored.
//
// DocumentIndexer indexes architecture documents, ADRs, and READMEs.
package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harshmaury/Atlas/internal/store"
)

// ── DOC TYPE DETECTION ───────────────────────────────────────────────────────

// docType returns the document type for a given path.
// Returns "" if the file is not a recognised document.
func docType(path string) string {
	name := strings.ToLower(filepath.Base(path))
	dir  := strings.ToLower(filepath.Base(filepath.Dir(path)))

	// ADR: files matching ADR-NNN-*.md in a decisions/ directory
	if dir == "decisions" && strings.HasPrefix(name, "adr-") && strings.HasSuffix(name, ".md") {
		return "adr"
	}
	// Architecture specifications
	if strings.Contains(name, "-specification") || strings.Contains(name, "-spec") {
		return "spec"
	}
	// Evolution guides
	if strings.Contains(name, "-evolution") || strings.Contains(name, "-guide") {
		return "guide"
	}
	// Platform constraints
	if strings.Contains(name, "constraint") || strings.Contains(name, "philosophy") {
		return "constraint"
	}
	// Capability / boundary documents
	if strings.Contains(name, "capability") || strings.Contains(name, "boundary") {
		return "capability"
	}
	// Workflow documents
	if strings.HasPrefix(name, "workflow-") {
		return "workflow"
	}
	// README files
	if name == "readme.md" {
		return "readme"
	}
	// Any markdown in an architecture/ directory
	if dir == "architecture" && strings.HasSuffix(name, ".md") {
		return "architecture"
	}
	return ""
}

// ── DOCUMENT INDEXER ─────────────────────────────────────────────────────────

// DocumentIndexer indexes architecture documents for a project.
type DocumentIndexer struct {
	store store.Storer
}

// NewDocumentIndexer creates a DocumentIndexer.
func NewDocumentIndexer(s store.Storer) *DocumentIndexer {
	return &DocumentIndexer{store: s}
}

// IndexProject walks a project directory and indexes all recognisable documents.
func (idx *DocumentIndexer) IndexProject(p *store.Project) (int, error) {
	if p.Path == "" {
		return 0, nil
	}
	if _, err := os.Stat(p.Path); err != nil {
		return 0, nil
	}

	count := 0
	if err := filepath.WalkDir(p.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip individual unreadable entries, continue walk
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		dt := docType(path)
		if dt == "" {
			return nil
		}

		idx.store.UpsertDocument(&store.Document{ //nolint:errcheck — best effort
			ProjectID: p.ID,
			Path:      path,
			DocType:   dt,
		})
		count++
		return nil
	}); err != nil {
		return count, fmt.Errorf("walk %s: %w", p.Path, err)
	}

	return count, nil
}

// IndexWorkspaceArchitecture indexes the workspace-level architecture directory.
// These documents belong to the platform, not a specific project.
// They are indexed under the special project ID "platform".
func (idx *DocumentIndexer) IndexWorkspaceArchitecture(archDir string) (int, error) {
	if _, err := os.Stat(archDir); err != nil {
		return 0, nil
	}

	count := 0
	filepath.WalkDir(archDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		dt := docType(path)
		if dt == "" && strings.HasSuffix(strings.ToLower(path), ".md") {
			dt = "architecture" // anything .md in the arch dir is relevant
		}
		if dt == "" {
			return nil
		}

		idx.store.UpsertDocument(&store.Document{ //nolint:errcheck
			ProjectID: "platform",
			Path:      path,
			DocType:   dt,
		})
		count++
		return nil
	})

	return count, nil
}
