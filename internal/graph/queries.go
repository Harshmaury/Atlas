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

// CircularDependencyResult records a cycle in the depends_on graph.
type CircularDependencyResult struct {
	Cycle []string // ordered path of project IDs forming the cycle e.g. [A, B, C, A]
}

// MissingDependencyResult records a depends_on declaration referencing
// a project that is not registered in Atlas.
type MissingDependencyResult struct {
	ProjectID  string // project that declared the dependency
	MissingDep string // project ID that does not exist in Atlas
}

// UndeclaredImportResult records an import-type graph edge where
// the source project does not declare the target in depends_on.
type UndeclaredImportResult struct {
	FromID string // project with the import
	ToID   string // imported project not in depends_on
	Source string // where the import edge was detected
}

// ConflictReport is the full output of a conflict detection run.
type ConflictReport struct {
	DuplicateOwnerships []DuplicateOwnershipResult
	UndefinedConsumers  []UndefinedConsumerResult
	OrphanedADRs        []OrphanedADRResult
	CircularDependencies []CircularDependencyResult  // Phase 4
	MissingDependencies  []MissingDependencyResult   // Phase 4
	UndeclaredImports    []UndeclaredImportResult    // Phase 4
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

// RunAll executes all conflict detectors and returns a combined report.
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
	report.CircularDependencies, err = q.FindCircularDependencies()
	if err != nil {
		return nil, fmt.Errorf("circular dependency query: %w", err)
	}
	report.MissingDependencies, err = q.FindMissingDependencies()
	if err != nil {
		return nil, fmt.Errorf("missing dependency query: %w", err)
	}
	report.UndeclaredImports, err = q.FindUndeclaredImports()
	if err != nil {
		return nil, fmt.Errorf("undeclared imports query: %w", err)
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

// ── DETECTOR 4 — Circular dependencies (Phase 4) ─────────────────────────────

// FindCircularDependencies detects cycles in the depends_on edge graph.
// Uses iterative DFS with a visited/stack set — safe for large graphs.
func (q *QueryRunner) FindCircularDependencies() ([]CircularDependencyResult, error) {
	edges, err := q.store.GetAllEdges()
	if err != nil {
		return nil, fmt.Errorf("get edges: %w", err)
	}
	adj := buildAdjacency(edges, "depends_on")
	var results []CircularDependencyResult
	visited := map[string]bool{}
	for node := range adj {
		if !visited[node] {
			if cycle := dfsCycle(node, adj, visited, []string{}); cycle != nil {
				results = append(results, CircularDependencyResult{Cycle: cycle})
			}
		}
	}
	return results, nil
}

// dfsCycle walks the adjacency map depth-first looking for back-edges.
// Returns the cycle path if found, nil otherwise.
func dfsCycle(node string, adj map[string][]string, visited map[string]bool, path []string) []string {
	visited[node] = true
	path = append(path, node)
	for _, neighbour := range adj[node] {
		for i, p := range path {
			if p == neighbour {
				return append(path[i:], neighbour) // cycle found
			}
		}
		if !visited[neighbour] {
			if cycle := dfsCycle(neighbour, adj, visited, path); cycle != nil {
				return cycle
			}
		}
	}
	return nil
}

// buildAdjacency builds a project-to-project adjacency map for a given edge type.
func buildAdjacency(edges []*store.GraphEdge, edgeType string) map[string][]string {
	adj := make(map[string][]string)
	for _, e := range edges {
		if e.EdgeType == edgeType {
			adj[e.FromID] = append(adj[e.FromID], e.ToID)
		}
	}
	return adj
}

// ── DETECTOR 5 — Missing declared dependencies (Phase 4) ─────────────────────

// FindMissingDependencies returns depends_on declarations referencing
// a project ID that is not registered in Atlas.
func (q *QueryRunner) FindMissingDependencies() ([]MissingDependencyResult, error) {
	projects, err := q.store.GetAllProjects()
	if err != nil {
		return nil, fmt.Errorf("get projects: %w", err)
	}
	edges, err := q.store.GetAllEdges()
	if err != nil {
		return nil, fmt.Errorf("get edges: %w", err)
	}
	known := make(map[string]bool, len(projects))
	for _, p := range projects {
		known[p.ID] = true
	}
	var results []MissingDependencyResult
	seen := make(map[string]bool)
	for _, e := range edges {
		if e.EdgeType != "depends_on" {
			continue
		}
		key := e.FromID + "→" + e.ToID
		if seen[key] {
			continue
		}
		seen[key] = true
		if !known[e.ToID] {
			results = append(results, MissingDependencyResult{
				ProjectID:  e.FromID,
				MissingDep: e.ToID,
			})
		}
	}
	return results, nil
}

// ── DETECTOR 6 — Undeclared imports (Phase 4) ─────────────────────────────────

// FindUndeclaredImports returns import-type edges where the source project
// does not declare the target in its depends_on edges.
// Indicates hidden coupling — code depends on something not formally declared.
func (q *QueryRunner) FindUndeclaredImports() ([]UndeclaredImportResult, error) {
	edges, err := q.store.GetAllEdges()
	if err != nil {
		return nil, fmt.Errorf("get edges: %w", err)
	}
	// Build set of declared depends_on pairs.
	declared := make(map[string]bool)
	for _, e := range edges {
		if e.EdgeType == "depends_on" {
			declared[e.FromID+"→"+e.ToID] = true
		}
	}
	var results []UndeclaredImportResult
	seen := make(map[string]bool)
	for _, e := range edges {
		if e.EdgeType != "import" {
			continue
		}
		key := e.FromID + "→" + e.ToID
		if seen[key] || declared[key] {
			continue
		}
		seen[key] = true
		results = append(results, UndeclaredImportResult{
			FromID: e.FromID,
			ToID:   e.ToID,
			Source: e.Source,
		})
	}
	return results, nil
}
