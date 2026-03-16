// @atlas-project: atlas
// @atlas-path: internal/discovery/scanner.go
// AT-H-04: manifestLanguage iteration order is now deterministic.
//
// Phase 3 (ADR-009): scanner now validates nexus.yaml for each discovered
// project and sets status=verified|unverified accordingly.
// Heuristic detection (manifest files, directory names) is demoted to
// discovery hints — these projects receive status=unverified.
// Only projects with a valid nexus.yaml receive status=verified.
//
// Package discovery walks the workspace and detects projects.
// It supplements the Nexus project registry (ADR-001) with locally
// detected projects that may not yet be registered with Nexus.
package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harshmaury/Atlas/internal/store"
	"github.com/Harshmaury/Atlas/internal/validator"
	nexusclient "github.com/Harshmaury/Atlas/internal/nexus"
)

// ── LANGUAGE DETECTION ───────────────────────────────────────────────────────

type manifestEntry struct {
	filename string
	language string
}

// manifestPriority defines manifest filenames in detection precedence order.
// AT-H-04: ordered slice guarantees deterministic language detection.
var manifestPriority = []manifestEntry{
	{"go.mod",           "go"},
	{"Cargo.toml",       "rust"},
	{"pyproject.toml",   "python"},
	{"requirements.txt", "python"},
	{"package.json",     "node"},
}

// extensionLanguage maps source file extensions to languages.
var extensionLanguage = map[string]string{
	".go":   "go",
	".py":   "python",
	".ts":   "node",
	".js":   "node",
	".tsx":  "node",
	".jsx":  "node",
	".rs":   "rust",
	".cs":   "dotnet",
	".java": "java",
	".kt":   "kotlin",
	".rb":   "ruby",
	".c":    "c",
	".cpp":  "cpp",
	".h":    "c",
	".md":   "markdown",
	".yaml": "yaml",
	".yml":  "yaml",
	".json": "json",
	".toml": "toml",
	".sh":   "shell",
}

// LanguageForExtension returns the language for a file extension, or "".
func LanguageForExtension(ext string) string {
	return extensionLanguage[strings.ToLower(ext)]
}

// ── SCANNER ──────────────────────────────────────────────────────────────────

// Scanner discovers projects in the workspace.
type Scanner struct {
	workspaceRoot string
	maxDepth      int
}

// NewScanner creates a Scanner for the given workspace root.
func NewScanner(workspaceRoot string) *Scanner {
	return &Scanner{workspaceRoot: workspaceRoot, maxDepth: 5}
}

// ScanResult is one discovered project.
// Phase 3: Status is always set — "verified" or "unverified".
type ScanResult struct {
	ID               string
	Name             string
	Path             string
	Language         string
	Type             string
	Source           string // "nexus" | "detected"
	Status           string // "verified" | "unverified"
	CapabilitiesJSON string // JSON array from nexus.yaml
	DependsOnJSON    string // JSON array from nexus.yaml
}

// ScanWorkspace walks the workspace and returns all discovered projects.
// Phase 3: each result has Status set based on nexus.yaml validation.
func (s *Scanner) ScanWorkspace() ([]*ScanResult, error) {
	var results []*ScanResult
	seen := map[string]bool{}

	err := filepath.WalkDir(s.workspaceRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if isSkippedDir(d.Name()) {
			return filepath.SkipDir
		}

		depth := strings.Count(strings.TrimPrefix(path, s.workspaceRoot), string(filepath.Separator))
		if depth > s.maxDepth {
			return filepath.SkipDir
		}

		if seen[path] {
			return nil
		}

		// Phase 3: always try nexus.yaml first — it is the authoritative descriptor.
		result := tryNexusYAML(path)
		if result != nil {
			seen[path] = true
			results = append(results, result)
			return filepath.SkipDir
		}

		// Fall back to heuristic manifest detection — produces unverified status.
		for _, m := range manifestPriority {
			if _, err := os.Stat(filepath.Join(path, m.filename)); err == nil {
				seen[path] = true
				name := filepath.Base(path)
				results = append(results, &ScanResult{
					ID:               strings.ToLower(name),
					Name:             name,
					Path:             path,
					Language:         m.language,
					Type:             "project",
					Source:           "detected",
					Status:           "unverified",
					CapabilitiesJSON: "[]",
					DependsOnJSON:    "[]",
				})
				return filepath.SkipDir
			}
		}

		return nil
	})

	return results, err
}

// tryNexusYAML attempts to read and validate nexus.yaml in projectPath.
// Returns a ScanResult with verified/unverified status, or nil if no
// nexus.yaml exists and no heuristic match was found at all.
func tryNexusYAML(projectPath string) *ScanResult {
	result := validator.ValidateDir(projectPath)

	// No file at all — let heuristic detection handle it.
	if len(result.Errors) == 1 && strings.Contains(result.Errors[0], "not found") {
		return nil
	}

	// File exists but failed to parse or validate — still index as unverified.
	if !result.Valid || result.Descriptor == nil {
		name := filepath.Base(projectPath)
		return &ScanResult{
			ID:               strings.ToLower(name),
			Name:             name,
			Path:             projectPath,
			Source:           "detected",
			Status:           "unverified",
			CapabilitiesJSON: "[]",
			DependsOnJSON:    "[]",
		}
	}

	d := result.Descriptor
	id := d.ID
	if id == "" {
		id = strings.ToLower(strings.ReplaceAll(d.Name, " ", "-"))
	}

	capsJSON := marshalStringSlice(d.Capabilities)
	depsJSON := marshalStringSlice(d.DependsOn)

	return &ScanResult{
		ID:               id,
		Name:             d.Name,
		Path:             projectPath,
		Language:         d.Language,
		Type:             d.Type,
		Source:           "detected",
		Status:           "verified",
		CapabilitiesJSON: capsJSON,
		DependsOnJSON:    depsJSON,
	}
}

// marshalStringSlice returns a JSON array string for a string slice.
// Returns "[]" for nil or empty slices.
func marshalStringSlice(s []string) string {
	if len(s) == 0 {
		return "[]"
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// MergeWithNexus combines scan results with the authoritative Nexus project list.
// Nexus projects take precedence over locally detected ones (ADR-001).
// Phase 3: Nexus projects are validated against nexus.yaml to get verified status.
func MergeWithNexus(scanned []*ScanResult, nexusProjects []*nexusclient.NexusProject) []*store.Project {
	merged := make(map[string]*store.Project)

	// Add scanned projects first.
	for _, s := range scanned {
		merged[s.ID] = &store.Project{
			ID:               s.ID,
			Name:             s.Name,
			Path:             s.Path,
			Language:         s.Language,
			Type:             s.Type,
			Source:           s.Source,
			Status:           s.Status,
			CapabilitiesJSON: s.CapabilitiesJSON,
			DependsOnJSON:    s.DependsOnJSON,
		}
	}

	// Nexus projects overwrite detected ones — Nexus is authoritative (ADR-001).
	// Phase 3: validate nexus.yaml at the registered path to get verified status.
	for _, n := range nexusProjects {
		vr := validator.ValidateDir(n.Path)
		status := vr.StatusString()

		capsJSON := "[]"
		depsJSON := "[]"
		if vr.Valid && vr.Descriptor != nil {
			capsJSON = marshalStringSlice(vr.Descriptor.Capabilities)
			depsJSON = marshalStringSlice(vr.Descriptor.DependsOn)
		}

		merged[n.ID] = &store.Project{
			ID:               n.ID,
			Name:             n.Name,
			Path:             n.Path,
			Language:         n.Language,
			Type:             n.ProjectType,
			Source:           "nexus",
			Status:           status,
			CapabilitiesJSON: capsJSON,
			DependsOnJSON:    depsJSON,
		}
	}

	projects := make([]*store.Project, 0, len(merged))
	for _, p := range merged {
		projects = append(projects, p)
	}
	return projects
}

// ── HELPERS ──────────────────────────────────────────────────────────────────

func isSkippedDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".cache", "__pycache__",
		"dist", "build", "target", "bin", ".nexus":
		return true
	}
	return strings.HasPrefix(name, ".")
}
