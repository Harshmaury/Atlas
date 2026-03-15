// @atlas-project: atlas
// @atlas-path: internal/capability/indexer.go
// CapabilityIndexer walks all indexed architecture documents and extracts
// structured capability claims using the parser, then persists them to the store.
//
// Run order (enforced by main.go Phase 2 wiring):
//   1. Document indexer runs first — populates the documents table
//   2. CapabilityIndexer runs after — reads documents table, parses each file
//
// Re-indexing a document:
//   DeleteCapabilitiesByDoc is called before re-parsing so stale claims
//   from a modified document are replaced cleanly.
package capability

import (
	"fmt"
	"log"

	"github.com/Harshmaury/Atlas/internal/store"
)

// ── INDEXER ───────────────────────────────────────────────────────────────────

// Indexer extracts capability claims from indexed architecture documents
// and persists them to the store.
type Indexer struct {
	store  store.Storer
	logger *log.Logger
}

// NewIndexer creates an Indexer.
func NewIndexer(s store.Storer, logger *log.Logger) *Indexer {
	return &Indexer{store: s, logger: logger}
}

// IndexResult summarises a completed capability indexing run.
type IndexResult struct {
	DocsScanned  int
	DocsIndexed  int
	ClaimsFound  int
	ClaimsStored int
	Errors       []string
}

// IndexAll walks all documents in the store and extracts capability claims.
// Documents that are not parseable (wrong type) are silently skipped.
// Errors on individual documents are recorded but do not stop the run.
func (idx *Indexer) IndexAll() (*IndexResult, error) {
	projects, err := idx.store.GetAllProjects()
	if err != nil {
		return nil, fmt.Errorf("get projects: %w", err)
	}

	result := &IndexResult{}

	for _, p := range projects {
		docs, err := idx.store.GetDocumentsByProject(p.ID)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("get docs for %s: %v", p.ID, err))
			continue
		}

		for _, doc := range docs {
			result.DocsScanned++

			caps, err := ParseDocument(doc.Path, p.ID)
			if err != nil {
				result.Errors = append(result.Errors,
					fmt.Sprintf("parse %s: %v", doc.Path, err))
				continue
			}
			if len(caps) == 0 {
				continue
			}

			result.DocsIndexed++
			result.ClaimsFound += len(caps)

			// Clear stale claims from this document before re-persisting.
			if err := idx.store.DeleteCapabilitiesByDoc(doc.Path); err != nil {
				result.Errors = append(result.Errors,
					fmt.Sprintf("delete stale caps for %s: %v", doc.Path, err))
				continue
			}

			for _, c := range caps {
				if err := idx.store.UpsertCapability(c); err != nil {
					result.Errors = append(result.Errors,
						fmt.Sprintf("upsert cap %s/%s: %v", c.Owner, c.Name, err))
					continue
				}
				result.ClaimsStored++
			}
		}
	}

	return result, nil
}

// IndexDocument re-indexes capability claims for a single document path.
// Used by the event subscriber when a specific file changes.
// projectID must match the project that owns the document.
func (idx *Indexer) IndexDocument(docPath, projectID string) (int, error) {
	caps, err := ParseDocument(docPath, projectID)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", docPath, err)
	}
	if len(caps) == 0 {
		return 0, nil
	}

	if err := idx.store.DeleteCapabilitiesByDoc(docPath); err != nil {
		return 0, fmt.Errorf("delete stale caps for %s: %w", docPath, err)
	}

	stored := 0
	for _, c := range caps {
		if err := idx.store.UpsertCapability(c); err != nil {
			idx.logger.Printf("WARNING: upsert capability %s/%s: %v", c.Owner, c.Name, err)
			continue
		}
		stored++
	}
	return stored, nil
}
