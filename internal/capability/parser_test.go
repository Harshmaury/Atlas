// @atlas-project: atlas
// @atlas-path: internal/capability/parser_test.go
package capability

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Harshmaury/Atlas/internal/store"
)

// ── HELPERS ──────────────────────────────────────────────────────────────────

// writeTempDoc writes content to a temp file with the given filename and
// returns the absolute path. Caller must defer os.Remove.
func writeTempDoc(t *testing.T, filename, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp doc: %v", err)
	}
	return path
}

// writeTempDocInDir writes content into a named subdirectory (e.g. "decisions").
func writeTempDocInDir(t *testing.T, subdir, filename, content string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), subdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp doc: %v", err)
	}
	return path
}

// ── classifyDoc ───────────────────────────────────────────────────────────────

func TestClassifyDoc(t *testing.T) {
	cases := []struct {
		path     string
		expected string
	}{
		{"decisions/ADR-001-project-registry-authority.md", "adr"},
		{"decisions/adr-004-forge-intent-model.md", "adr"},
		{"platform-capability-boundaries.md", "capability"},
		{"2_platform-capability-boundaries.md", "capability"},
		{"atlas-specification.md", "spec"},
		{"forge-spec.md", "spec"},
		{"README.md", ""},
		{"WORKFLOW-SESSION.md", ""},
		{"nexus-evolution-guide.md", ""},
	}

	for _, tc := range cases {
		got := classifyDoc(tc.path)
		if got != tc.expected {
			t.Errorf("classifyDoc(%q) = %q, want %q", tc.path, got, tc.expected)
		}
	}
}

// ── normaliseCapabilityName ───────────────────────────────────────────────────

func TestNormaliseCapabilityName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Project registry", "project-registry"},
		{"Project registry (ADR-001)", "project-registry"},
		{"Filesystem observation", "filesystem-observation"},
		{"Runtime providers", "runtime-providers"},
		{"  Source file indexing  ", "source-file-indexing"},
		{"AI context generation", "ai-context-generation"},
	}

	for _, tc := range cases {
		got := normaliseCapabilityName(tc.input)
		if got != tc.expected {
			t.Errorf("normalise(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ── Capability boundary doc ───────────────────────────────────────────────────

func TestParseCapabilityBoundaryDoc(t *testing.T) {
	content := `# Platform Capability Boundaries

| Capability                  | Nexus | Atlas | Forge |
|-----------------------------|-------|-------|-------|
| Project registry            | ✓     | ✗     | ✗     |
| Service state management    | ✓     | ✗     | ✗     |
| Workspace discovery         | ✗     | ✓     | ✗     |
| Source indexing             | ✗     | ✓     | ✗     |
| Command intake              | ✗     | ✗     | ✓     |
| Intent execution            | ✗     | ✗     | ✓     |
`
	path := writeTempDoc(t, "platform-capability-boundaries.md", content)
	caps, err := ParseDocument(path, "platform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(caps) != 6 {
		t.Fatalf("want 6 capabilities, got %d", len(caps))
	}

	// Check ownership assignment.
	byName := make(map[string]*capResult)
	for _, c := range caps {
		byName[c.Name] = &capResult{owner: c.Owner, domain: c.Domain}
	}

	checks := []struct {
		name   string
		owner  string
		domain string
	}{
		{"project-registry", "nexus", "Control"},
		{"service-state-management", "nexus", "Control"},
		{"workspace-discovery", "atlas", "Knowledge"},
		{"source-indexing", "atlas", "Knowledge"},
		{"command-intake", "forge", "Execution"},
		{"intent-execution", "forge", "Execution"},
	}

	for _, check := range checks {
		r, ok := byName[check.name]
		if !ok {
			t.Errorf("missing capability %q", check.name)
			continue
		}
		if r.owner != check.owner {
			t.Errorf("capability %q: owner = %q, want %q", check.name, r.owner, check.owner)
		}
		if r.domain != check.domain {
			t.Errorf("capability %q: domain = %q, want %q", check.name, r.domain, check.domain)
		}
	}
}

type capResult struct {
	owner  string
	domain string
}

// ── ADR parser ────────────────────────────────────────────────────────────────

func TestParseADR(t *testing.T) {
	content := `# ADR-001 — Project Registry Authority

## Context

Multiple components need project metadata.

## Decision

Nexus is the authoritative project registry.

Nexus owns all project registration and lifecycle.

## Implications

Atlas reads project information by querying the Nexus HTTP API.
`
	path := writeTempDocInDir(t, "decisions", "ADR-001-project-registry-authority.md", content)
	caps, err := ParseDocument(path, "platform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(caps) == 0 {
		t.Fatal("want at least 1 capability claim, got 0")
	}

	for _, c := range caps {
		if c.Owner != "nexus" {
			t.Errorf("expected owner nexus, got %q", c.Owner)
		}
		if c.Domain != "Control" {
			t.Errorf("expected domain Control, got %q", c.Domain)
		}
		if c.DocType != "adr" {
			t.Errorf("expected doctype adr, got %q", c.DocType)
		}
	}
}

// ── Specification parser ──────────────────────────────────────────────────────

func TestParseSpecification(t *testing.T) {
	content := `# Atlas Architecture Specification

## What Atlas Owns

- Workspace discovery and project detection
- Source file indexing
- Architecture document indexing
- AI context generation

## What Atlas Does Not Own

- Project registry authority (Nexus)
- Filesystem watchers (Nexus)
`
	path := writeTempDoc(t, "atlas-specification.md", content)
	caps, err := ParseDocument(path, "atlas")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(caps) != 4 {
		t.Fatalf("want 4 capabilities, got %d: %v", len(caps), capNames(caps))
	}

	for _, c := range caps {
		if c.Owner != "atlas" {
			t.Errorf("expected owner atlas, got %q", c.Owner)
		}
		if c.Domain != "Knowledge" {
			t.Errorf("expected domain Knowledge, got %q", c.Domain)
		}
		if c.DocType != "spec" {
			t.Errorf("expected doctype spec, got %q", c.DocType)
		}
	}
}

// ── Non-parseable documents ───────────────────────────────────────────────────

func TestParseDocument_UnknownDocType(t *testing.T) {
	path := writeTempDoc(t, "WORKFLOW-SESSION.md", "# WORKFLOW-SESSION\nsome content\n")
	caps, err := ParseDocument(path, "nexus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(caps) != 0 {
		t.Errorf("expected 0 caps for unknown doc type, got %d", len(caps))
	}
}

func TestParseDocument_MissingFile(t *testing.T) {
	caps, err := ParseDocument("/tmp/does-not-exist.md", "nexus")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(caps) != 0 {
		t.Errorf("expected 0 caps for missing file, got %d", len(caps))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func capNames(caps []*store.Capability) []string {
	names := make([]string, len(caps))
	for i, c := range caps {
		names[i] = c.Name
	}
	return names
}
