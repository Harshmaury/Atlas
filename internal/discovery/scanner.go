// @atlas-project: atlas
// @atlas-path: internal/discovery/scanner.go
// Package discovery walks the workspace and detects projects.
// It supplements the Nexus project registry (ADR-001) with locally
// detected projects that may not yet be registered with Nexus.
package discovery

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Harshmaury/Atlas/internal/store"
	nexusclient "github.com/Harshmaury/Atlas/internal/nexus"
	"gopkg.in/yaml.v3"
)

// ── LANGUAGE DETECTION ───────────────────────────────────────────────────────

// manifestLanguage maps project manifest filenames to the language they imply.
var manifestLanguage = map[string]string{
	"go.mod":        "go",
	"package.json":  "node",
	"Cargo.toml":    "rust",
	"pyproject.toml":"python",
	"requirements.txt": "python",
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

// ── NEXUS YAML ───────────────────────────────────────────────────────────────

// nexusManifest is the parsed .nexus.yaml structure.
type nexusManifest struct {
	Name     string `yaml:"name"`
	ID       string `yaml:"id"`
	Language string `yaml:"language"`
	Type     string `yaml:"type"`
}

func readNexusManifest(projectPath string) (*nexusManifest, error) {
	data, err := os.ReadFile(filepath.Join(projectPath, ".nexus.yaml"))
	if err != nil {
		return nil, err
	}
	var m nexusManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.ID == "" {
		m.ID = strings.ToLower(strings.ReplaceAll(m.Name, " ", "-"))
	}
	return &m, nil
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
type ScanResult struct {
	ID       string
	Name     string
	Path     string
	Language string
	Type     string
	Source   string // "nexus" | "detected"
}

// ScanWorkspace walks the workspace and returns all discovered projects.
// It does not consult Nexus — use MergeWithNexus to combine results.
func (s *Scanner) ScanWorkspace() ([]*ScanResult, error) {
	var results []*ScanResult
	seen := map[string]bool{}

	err := filepath.WalkDir(s.workspaceRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
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

		// .nexus.yaml is the strongest signal — use it preferentially.
		if m, err := readNexusManifest(path); err == nil && m.Name != "" {
			seen[path] = true
			results = append(results, &ScanResult{
				ID:       m.ID,
				Name:     m.Name,
				Path:     path,
				Language: m.Language,
				Type:     m.Type,
				Source:   "detected",
			})
			return filepath.SkipDir
		}

		// Fall back to language manifest detection.
		for manifest, lang := range manifestLanguage {
			if _, err := os.Stat(filepath.Join(path, manifest)); err == nil {
				seen[path] = true
				name := filepath.Base(path)
				results = append(results, &ScanResult{
					ID:       strings.ToLower(name),
					Name:     name,
					Path:     path,
					Language: lang,
					Type:     "project",
					Source:   "detected",
				})
				return filepath.SkipDir
			}
		}

		return nil
	})

	return results, err
}

// MergeWithNexus combines scan results with the authoritative Nexus project list.
// Nexus projects take precedence over locally detected ones (ADR-001).
func MergeWithNexus(scanned []*ScanResult, nexusProjects []*nexusclient.NexusProject) []*store.Project {
	merged := make(map[string]*store.Project)

	// Add scanned projects first.
	for _, s := range scanned {
		merged[s.ID] = &store.Project{
			ID:       s.ID,
			Name:     s.Name,
			Path:     s.Path,
			Language: s.Language,
			Type:     s.Type,
			Source:   "detected",
		}
	}

	// Nexus projects overwrite detected ones — Nexus is authoritative.
	for _, n := range nexusProjects {
		merged[n.ID] = &store.Project{
			ID:       n.ID,
			Name:     n.Name,
			Path:     n.Path,
			Language: n.Language,
			Type:     n.ProjectType,
			Source:   "nexus",
		}
	}

	projects := make([]*store.Project, 0, len(merged))
	for _, p := range merged {
		projects = append(projects, p)
	}
	return projects
}

// ── HELPERS ──────────────────────────────────────────────────────────────────

// isSkippedDir returns true for directories that should never be scanned.
func isSkippedDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".cache", "__pycache__",
		"dist", "build", "target", "bin", ".nexus":
		return true
	}
	return strings.HasPrefix(name, ".")
}
