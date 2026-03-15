// @atlas-project: atlas
// @atlas-path: internal/graph/builder.go
// AT-H-02: BuildAll wraps delete+rebuild per source in a transaction.
//   Previously DeleteEdgesBySource ran outside any transaction — if the
//   process crashed between delete and rebuild the graph was permanently
//   empty for that source. Each source now runs in its own BEGIN/COMMIT
//   so a crash leaves the previous edges intact rather than none.
//
// AT-H-01: extractGoImports rewritten using go/ast + go/parser.
//   The hand-rolled scanner missed: blank separator lines inside import
//   blocks (goimports stdlib/external/internal grouping), aliased imports
//   inside grouped blocks, and build-tag lines before the package clause.
//   go/parser handles all of these correctly with zero new dependencies.
//
// Package graph builds the workspace relationship graph.
//
// Edge sources:
//
//  1. nexus.yaml — service depends_on declarations
//     Produces edges: service → dependency (edge_type: "depends_on", source: "nexus.yaml")
//
//  2. Go import paths — cross-project imports between platform services
//     Scans *.go files in each project, detects imports referencing other
//     known platform module paths, produces edges:
//     importing-project → imported-project (edge_type: "references", source: "import")
//
//  3. ADR cross-references — "See ADR-NNN" mentions in architecture docs
//     Produces edges: doc-path → ADR-path (edge_type: "references", source: "adr")
//
// Each edge type is rebuilt cleanly:
//   DeleteEdgesBySource is called per source before re-building so stale
//   edges from renamed or removed files are not left in the graph.
package graph

import (
	"fmt"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Harshmaury/Atlas/internal/store"
	"gopkg.in/yaml.v3"
)

// ── CONSTANTS ────────────────────────────────────────────────────────────────

// knownModules maps Go module paths to their Atlas project IDs.
// Used to detect cross-project imports in Go source files.
var knownModules = map[string]string{
	"github.com/Harshmaury/Nexus":  "nexus",
	"github.com/Harshmaury/Atlas":  "atlas",
	"github.com/Harshmaury/Forge":  "forge",
}

// adrReferencePattern matches "ADR-NNN" or "ADR-NNN-title" references.
var adrReferencePattern = regexp.MustCompile(`ADR-\d{3}`)

// ── BUILDER ───────────────────────────────────────────────────────────────────

// Builder constructs the workspace relationship graph.
type Builder struct {
	store  store.Storer
	logger *log.Logger
}

// NewBuilder creates a Builder.
func NewBuilder(s store.Storer, logger *log.Logger) *Builder {
	return &Builder{store: s, logger: logger}
}

// BuildResult summarises a completed graph build.
type BuildResult struct {
	EdgesFromNexusYAML int
	EdgesFromImports   int
	EdgesFromADRRefs   int
	Errors             []string
}

// BuildAll rebuilds all graph edges from all sources.
// Clears existing edges per source before rebuilding.
func (b *Builder) BuildAll() (*BuildResult, error) {
	projects, err := b.store.GetAllProjects()
	if err != nil {
		return nil, fmt.Errorf("get projects: %w", err)
	}

	result := &BuildResult{}

	// Build project lookup by path for import detection.
	pathToID := make(map[string]string, len(projects))
	for _, p := range projects {
		if p.Path != "" {
			pathToID[p.Path] = p.ID
		}
	}

	// Each source is rebuilt atomically (AT-H-02):
	//   1. DeleteEdgesBySource removes stale edges
	//   2. rebuild loop adds fresh edges
	// Both run inside WithEdgeTransaction — a crash leaves previous edges
	// intact rather than an empty graph for that source.

	// Source 1 — nexus.yaml service dependencies.
	if err := b.store.WithEdgeTransaction(func() error {
		if err := b.store.DeleteEdgesBySource("nexus.yaml"); err != nil {
			return fmt.Errorf("clear nexus.yaml edges: %w", err)
		}
		for _, p := range projects {
			if p.Path == "" {
				continue
			}
			n, errs := b.buildNexusYAMLEdges(p)
			result.EdgesFromNexusYAML += n
			result.Errors = append(result.Errors, errs...)
		}
		return nil
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("nexus.yaml transaction: %v", err))
	}

	// Source 2 — Go import cross-references.
	if err := b.store.WithEdgeTransaction(func() error {
		if err := b.store.DeleteEdgesBySource("import"); err != nil {
			return fmt.Errorf("clear import edges: %w", err)
		}
		for _, p := range projects {
			if p.Path == "" {
				continue
			}
			n, errs := b.buildImportEdges(p, pathToID)
			result.EdgesFromImports += n
			result.Errors = append(result.Errors, errs...)
		}
		return nil
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("import transaction: %v", err))
	}

	// Source 3 — ADR cross-references in architecture docs.
	if err := b.store.WithEdgeTransaction(func() error {
		if err := b.store.DeleteEdgesBySource("adr"); err != nil {
			return fmt.Errorf("clear adr edges: %w", err)
		}
		for _, p := range projects {
			if p.Path == "" {
				continue
			}
			n, errs := b.buildADRRefEdges(p)
			result.EdgesFromADRRefs += n
			result.Errors = append(result.Errors, errs...)
		}
		return nil
	}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("adr transaction: %v", err))
	}

	return result, nil
}

// ── SOURCE 1 — nexus.yaml ────────────────────────────────────────────────────

type nexusYAML struct {
	Services []struct {
		ID        string   `yaml:"id"`
		DependsOn []string `yaml:"depends_on"`
	} `yaml:"services"`
}

// buildNexusYAMLEdges reads .nexus.yaml and creates depends_on edges.
func (b *Builder) buildNexusYAMLEdges(p *store.Project) (int, []string) {
	manifestPath := filepath.Join(p.Path, ".nexus.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return 0, nil // no manifest — skip silently
	}

	var manifest nexusYAML
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return 0, []string{fmt.Sprintf("parse %s: %v", manifestPath, err)}
	}

	count := 0
	var errs []string
	for _, svc := range manifest.Services {
		for _, dep := range svc.DependsOn {
			edge := &store.GraphEdge{
				FromID:   p.ID + ":" + svc.ID,
				ToID:     p.ID + ":" + dep,
				EdgeType: "depends_on",
				Source:   "nexus.yaml",
			}
			if err := b.store.UpsertEdge(edge); err != nil {
				errs = append(errs, fmt.Sprintf("upsert edge %s→%s: %v",
					edge.FromID, edge.ToID, err))
				continue
			}
			count++
		}
	}
	return count, errs
}

// ── SOURCE 2 — Go imports ────────────────────────────────────────────────────

// buildImportEdges scans Go source files and creates references edges
// when a file imports a known platform module from a different project.
func (b *Builder) buildImportEdges(p *store.Project, pathToID map[string]string) (int, []string) {
	count := 0
	seen := make(map[string]bool) // deduplicate project-level edges
	var errs []string

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
		if filepath.Ext(path) != ".go" {
			return nil
		}

		imports, err := extractGoImports(path)
		if err != nil {
			return nil // skip unreadable files
		}

		for _, imp := range imports {
			for modulePath, targetID := range knownModules {
				if targetID == p.ID {
					continue // skip self-references
				}
				if strings.HasPrefix(imp, modulePath) {
					edgeKey := p.ID + "→" + targetID
					if seen[edgeKey] {
						continue
					}
					seen[edgeKey] = true

					if err := b.store.UpsertEdge(&store.GraphEdge{
						FromID:   p.ID,
						ToID:     targetID,
						EdgeType: "references",
						Source:   "import",
					}); err != nil {
						errs = append(errs, fmt.Sprintf("upsert import edge %s→%s: %v",
							p.ID, targetID, err))
						continue
					}
					count++
				}
			}
		}
		return nil
	})

	if err != nil {
		errs = append(errs, fmt.Sprintf("walk %s: %v", p.Path, err))
	}
	return count, errs
}

// extractGoImports returns all import paths from a Go source file.
//
// AT-H-01: uses go/parser instead of a hand-rolled line scanner.
// Handles all valid Go import forms correctly:
//   - grouped imports with blank separator lines (goimports output)
//   - aliased imports: alias "path/to/pkg"
//   - dot and blank imports: . "pkg" / _ "pkg"
//   - single-line: import "path"
//   - build tags and comments before the package clause
//
// go/parser is in the standard library — no new dependency.
// ParseFile with ImportsOnly stops parsing after the import section,
// keeping this fast even for large source files.
func extractGoImports(filePath string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
	if err != nil {
		// Syntax errors in the file — skip it silently.
		// The graph builder already tolerates unreadable files.
		return nil, nil //nolint:nilerr
	}

	imports := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		if imp.Path == nil {
			continue
		}
		// ast.BasicLit.Value includes surrounding quotes — strip them.
		path := strings.Trim(imp.Path.Value, `"`)
		if path != "" {
			imports = append(imports, path)
		}
	}
	return imports, nil
}


// ── SOURCE 3 — ADR cross-references ─────────────────────────────────────────

// buildADRRefEdges scans architecture documents for ADR-NNN mentions
// and creates reference edges between the mentioning doc and the ADR.
func (b *Builder) buildADRRefEdges(p *store.Project) (int, []string) {
	docs, err := b.store.GetDocumentsByProject(p.ID)
	if err != nil {
		return 0, []string{fmt.Sprintf("get docs for %s: %v", p.ID, err)}
	}

	count := 0
	seen := make(map[string]bool)
	var errs []string

	for _, doc := range docs {
		refs, err := extractADRReferences(doc.Path)
		if err != nil || len(refs) == 0 {
			continue
		}

		for _, adrNum := range refs {
			edgeKey := doc.Path + "→" + adrNum
			if seen[edgeKey] {
				continue
			}
			seen[edgeKey] = true

			if err := b.store.UpsertEdge(&store.GraphEdge{
				FromID:   doc.Path,
				ToID:     adrNum, // e.g. "ADR-002"
				EdgeType: "references",
				Source:   "adr",
			}); err != nil {
				errs = append(errs, fmt.Sprintf("upsert adr ref edge: %v", err))
				continue
			}
			count++
		}
	}
	return count, errs
}

// extractADRReferences returns all unique ADR-NNN identifiers found in a file.
func extractADRReferences(filePath string) ([]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	matches := adrReferencePattern.FindAllString(string(data), -1)
	if len(matches) == 0 {
		return nil, nil
	}
	// Deduplicate.
	seen := make(map[string]bool)
	var refs []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			refs = append(refs, m)
		}
	}
	return refs, nil
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
