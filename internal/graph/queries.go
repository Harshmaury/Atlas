// @atlas-project: atlas
// @atlas-path: internal/graph/queries.go
// AT-Fix-03: FindOrphanedADRs rewritten.
//   1. N+1 eliminated — GetAllDocuments replaces per-project GetDocumentsByProject loop.
//   2. Self-reference false negative fixed — an ADR that mentions its own ID
//      in its content was previously excluded from orphan results incorrectly.
//      Now an edge only counts as a reference if FromID != the ADR's own path.
//
// Package graph — conflict detection queries.
//
// Three detectors, each returns a typed result slice:
//
//  1. DuplicateOwnership — capabilities claimed by more than one service
//     Source: capabilities table, grouped by name, count owners > 1
//
//  2. UndefinedConsumers — graph edges referencing a project that has
//     no indexed capabilities (may indicate an unregistered or future service)
//     Source: graph_edges WHERE edge_type = "references", cross-check projects
//
//  3. OrphanedADRs — ADR documents that are not referenced by any other doc
//     Source: documents WHERE doc_type = "adr", cross-check graph_edges
//
// All detectors are read-only. They never write to the store.
package graph

import (
	"fmt"

	"github.com/Harshmaury/Atlas/internal/store"
)

// ── RESULT TYPES ─────────────────────────────────────────────────────────────

// DuplicateOwnershipResult records a capability claimed by multiple services.
type DuplicateOwnershipResult struct {
	CapabilityName string
	Owners         []string // all services claiming this capability
	DocPaths       []string // source documents for each claim
}

// UndefinedConsumerResult records a reference edge pointing to an unknown target.
type UndefinedConsumerResult struct {
	FromID   string // project or doc making the reference
	ToID     string // target that has no indexed capabilities
	EdgeType string
	Source   string
}

// OrphanedADRResult records an ADR document with no incoming references.
type OrphanedADRResult struct {
	DocPath string
	Project string
}

// ConflictReport is the full output of a conflict detection run.
type ConflictReport struct {
	DuplicateOwnerships []DuplicateOwnershipResult
	UndefinedConsumers  []UndefinedConsumerResult
	OrphanedADRs        []OrphanedADRResult
}

// ── QUERIES ───────────────────────────────────────────────────────────────────

// QueryRunner runs all conflict detection queries and returns a combined report.
type QueryRunner struct {
	store store.Storer
}

// NewQueryRunner creates a QueryRunner.
func NewQueryRunner(s store.Storer) *QueryRunner {
	return &QueryRunner{store: s}
}

// RunAll executes all three conflict detectors and returns a combined report.
func (q *QueryRunner) RunAll() (*ConflictReport, error) {
	report := &ConflictReport{}
	var err error

	report.DuplicateOwnerships, err = q.FindDuplicateOwnerships()
	if err != nil {
		return nil, fmt.Errorf("duplicate ownership query: %w", err)
	}

	report.UndefinedConsumers, err = q.FindUndefinedConsumers()
	if err != nil {
		return nil, fmt.Errorf("undefined consumers query: %w", err)
	}

	report.OrphanedADRs, err = q.FindOrphanedADRs()
	if err != nil {
		return nil, fmt.Errorf("orphaned ADRs query: %w", err)
	}

	return report, nil
}

// ── DETECTOR 1 — Duplicate ownership ─────────────────────────────────────────

// FindDuplicateOwnerships returns capabilities claimed by more than one service.
// A clean architecture has zero results here.
func (q *QueryRunner) FindDuplicateOwnerships() ([]DuplicateOwnershipResult, error) {
	caps, err := q.store.GetAllCapabilities()
	if err != nil {
		return nil, fmt.Errorf("get capabilities: %w", err)
	}

	// Group by capability name.
	type entry struct {
		owners   []string
		docPaths []string
	}
	byName := make(map[string]*entry)

	for _, c := range caps {
		e, ok := byName[c.Name]
		if !ok {
			e = &entry{}
			byName[c.Name] = e
		}
		e.owners = append(e.owners, c.Owner)
		e.docPaths = append(e.docPaths, c.DocPath)
	}

	var results []DuplicateOwnershipResult
	for name, e := range byName {
		if len(uniqueStrings(e.owners)) > 1 {
			results = append(results, DuplicateOwnershipResult{
				CapabilityName: name,
				Owners:         uniqueStrings(e.owners),
				DocPaths:       e.docPaths,
			})
		}
	}
	return results, nil
}

// ── DETECTOR 2 — Undefined consumers ─────────────────────────────────────────

// FindUndefinedConsumers returns reference edges pointing to project IDs
// that have no indexed capabilities — suggesting an unregistered service
// or a stale reference.
func (q *QueryRunner) FindUndefinedConsumers() ([]UndefinedConsumerResult, error) {
	caps, err := q.store.GetAllCapabilities()
	if err != nil {
		return nil, fmt.Errorf("get capabilities: %w", err)
	}

	// Build set of known capability owners.
	knownOwners := make(map[string]bool)
	for _, c := range caps {
		knownOwners[c.Owner] = true
	}

	edges, err := q.store.GetAllEdges()
	if err != nil {
		return nil, fmt.Errorf("get edges: %w", err)
	}

	var results []UndefinedConsumerResult
	seen := make(map[string]bool)

	for _, e := range edges {
		if e.EdgeType != "references" {
			continue
		}
		// toID may be "ADR-NNN" (not a project) — skip those.
		if isADRIdentifier(e.ToID) {
			continue
		}
		key := e.FromID + "→" + e.ToID
		if seen[key] {
			continue
		}
		seen[key] = true

		if !knownOwners[e.ToID] {
			results = append(results, UndefinedConsumerResult{
				FromID:   e.FromID,
				ToID:     e.ToID,
				EdgeType: e.EdgeType,
				Source:   e.Source,
			})
		}
	}
	return results, nil
}

// ── DETECTOR 3 — Orphaned ADRs ────────────────────────────────────────────────

// FindOrphanedADRs returns ADR documents that are not referenced by any
// other document in the graph. An ADR with no references may be stale or
// superseded without a formal notation.
//
// AT-Fix-03 changes:
//  1. Uses GetAllDocuments (single query) instead of per-project
//     GetDocumentsByProject (N queries) — eliminates the N+1 pattern.
//  2. Self-reference false negative fixed: an edge only counts as an
//     external reference if e.FromID != the ADR document's own path.
//     Previously, an ADR that mentioned its own ID (e.g. "See ADR-003")
//     in its content was stored as an edge ADR-003.md → ADR-003 and
//     incorrectly treated as "referenced", hiding it from orphan results.
func (q *QueryRunner) FindOrphanedADRs() ([]OrphanedADRResult, error) {
	// Single query — no N+1 per project (AT-Fix-03).
	allDocs, err := q.store.GetAllDocuments()
	if err != nil {
		return nil, fmt.Errorf("get all documents: %w", err)
	}

	type adrDoc struct {
		path      string
		projectID string
		adrID     string // e.g. "ADR-003" extracted from path
	}
	var adrs []adrDoc
	for _, d := range allDocs {
		if d.DocType != "adr" {
			continue
		}
		id := extractADRID(d.Path)
		if id == "" {
			continue
		}
		adrs = append(adrs, adrDoc{path: d.Path, projectID: d.ProjectID, adrID: id})
	}
	if len(adrs) == 0 {
		return nil, nil
	}

	edges, err := q.store.GetAllEdges()
	if err != nil {
		return nil, fmt.Errorf("get edges: %w", err)
	}

	// referencedBy maps ADR-NNN → set of referencing document paths.
	// We use a set so that an ADR referenced by multiple docs is not double-counted.
	referencedBy := make(map[string]map[string]bool)
	for _, e := range edges {
		if e.EdgeType != "references" || !isADRIdentifier(e.ToID) {
			continue
		}
		if referencedBy[e.ToID] == nil {
			referencedBy[e.ToID] = make(map[string]bool)
		}
		referencedBy[e.ToID][e.FromID] = true
	}

	var results []OrphanedADRResult
	for _, adr := range adrs {
		refs := referencedBy[adr.adrID]
		// An ADR is orphaned only if no *other* document references it.
		// Self-references (the ADR mentioning its own ID) don't count (AT-Fix-03).
		external := 0
		for from := range refs {
			if from != adr.path {
				external++
			}		}
		if external == 0 {
			results = append(results, OrphanedADRResult{
				DocPath: adr.path,
				Project: adr.projectID,
			})
		}
	}
	return results, nil
}

// ── HELPERS ──────────────────────────────────────────────────────────────────

// uniqueStrings returns a deduplicated slice preserving order.
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// isADRIdentifier returns true for strings like "ADR-001", "ADR-004".
func isADRIdentifier(s string) bool {
	return adrReferencePattern.MatchString(s) && len(s) == len("ADR-000")
}

// extractADRID returns the ADR-NNN identifier from a file path, or "".
// "decisions/ADR-002-workspace-observation.md" → "ADR-002"
func extractADRID(path string) string {
	matches := adrReferencePattern.FindString(path)
	return matches
}
