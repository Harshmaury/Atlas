// @atlas-project: atlas
// @atlas-path: internal/context/generator.go
// Package context generates structured workspace summaries for AI sessions.
// The output is designed to orient an AI system to the workspace without
// requiring it to scan the filesystem or infer project structure.
package context

import (
	"time"

	"github.com/Harshmaury/Atlas/internal/store"
)

// WorkspaceContext is the AI-ready workspace summary.
type WorkspaceContext struct {
	WorkspaceRoot  string            `json:"workspace_root"`
	GeneratedAt    time.Time         `json:"generated_at"`
	Projects       []*ProjectSummary `json:"projects"`
	Languages      []string          `json:"languages"`
	TotalFiles     int               `json:"total_files"`
	TotalDocuments int               `json:"total_documents"`
	ArchitectureDocs []DocSummary    `json:"architecture_docs"`
}

// ProjectSummary is a condensed view of one project.
type ProjectSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Language  string `json:"language"`
	Type      string `json:"type"`
	Source    string `json:"source"`
	FileCount int    `json:"file_count"`
	DocCount  int    `json:"doc_count"`
}

// DocSummary is a condensed view of one architecture document.
type DocSummary struct {
	Path    string `json:"path"`
	DocType string `json:"doc_type"`
	Project string `json:"project"`
}

// Generator produces workspace context snapshots.
type Generator struct {
	store         store.Storer
	workspaceRoot string
}

// New creates a Generator.
func New(s store.Storer, workspaceRoot string) *Generator {
	return &Generator{store: s, workspaceRoot: workspaceRoot}
}

// Generate produces a full workspace context snapshot.
func (g *Generator) Generate() (*WorkspaceContext, error) {
	projects, err := g.store.GetAllProjects()
	if err != nil {
		return nil, err
	}

	totalFiles, _ := g.store.CountFiles()
	totalDocs, _  := g.store.CountDocuments()

	// Build per-project summaries and collect unique languages.
	langSet := map[string]bool{}
	summaries := make([]*ProjectSummary, 0, len(projects))

	for _, p := range projects {
		files, _  := g.store.GetFilesByProject(p.ID)
		docs, _   := g.store.GetDocumentsByProject(p.ID)

		if p.Language != "" {
			langSet[p.Language] = true
		}

		summaries = append(summaries, &ProjectSummary{
			ID:        p.ID,
			Name:      p.Name,
			Path:      p.Path,
			Language:  p.Language,
			Type:      p.Type,
			Source:    p.Source,
			FileCount: len(files),
			DocCount:  len(docs),
		})
	}

	// Collect platform-level architecture documents.
	platformDocs, _ := g.store.GetDocumentsByProject("platform")
	archDocs := make([]DocSummary, 0, len(platformDocs))
	for _, d := range platformDocs {
		archDocs = append(archDocs, DocSummary{
			Path:    d.Path,
			DocType: d.DocType,
			Project: "platform",
		})
	}

	// Flatten language set.
	languages := make([]string, 0, len(langSet))
	for lang := range langSet {
		languages = append(languages, lang)
	}

	return &WorkspaceContext{
		WorkspaceRoot:    g.workspaceRoot,
		GeneratedAt:      time.Now().UTC(),
		Projects:         summaries,
		Languages:        languages,
		TotalFiles:       totalFiles,
		TotalDocuments:   totalDocs,
		ArchitectureDocs: archDocs,
	}, nil
}
