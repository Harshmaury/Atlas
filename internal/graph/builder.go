// @atlas-project: atlas
// @atlas-path: internal/graph/builder.go
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
	"bufio"
	"fmt"
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

	// Clear stale edges before rebuilding each source.
	for _, source := range []string{"nexus.yaml", "import", "adr"} {
		if err := b.store.DeleteEdgesBySource(source); err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("clear edges source=%s: %v", source, err))
		}
	}

	for _, p := range projects {
		if p.Path == "" {
			continue
		}

		// Source 1 — nexus.yaml service dependencies.
		n, errs := b.buildNexusYAMLEdges(p)
		result.EdgesFromNexusYAML += n
		result.Errors = append(result.Errors, errs...)

		// Source 2 — Go import cross-references.
		n, errs = b.buildImportEdges(p, pathToID)
		result.EdgesFromImports += n
		result.Errors = append(result.Errors, errs...)

		// Source 3 — ADR cross-references in architecture docs.
		n, errs = b.buildADRRefEdges(p)
		result.EdgesFromADRRefs += n
		result.Errors = append(result.Errors, errs...)
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
func extractGoImports(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	inImportBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "import (" {
			inImportBlock = true
			continue
		}
		if inImportBlock && line == ")" {
			break
		}
		if inImportBlock {
			imp := strings.Trim(line, `"`)
			// Strip alias if present (e.g. `alias "path"`)
			if parts := strings.Fields(imp); len(parts) == 2 {
				imp = strings.Trim(parts[1], `"`)
			}
			if imp != "" && !strings.HasPrefix(imp, "//") {
				imports = append(imports, imp)
			}
			continue
		}
		// Single-line import: import "path"
		if strings.HasPrefix(line, `import "`) {
			imp := strings.TrimPrefix(line, `import "`)
			imp = strings.TrimSuffix(imp, `"`)
			imports = append(imports, imp)
		}
	}
	return imports, scanner.Err()
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
