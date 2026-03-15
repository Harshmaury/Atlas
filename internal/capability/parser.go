// @atlas-project: atlas
// @atlas-path: internal/capability/parser.go
// Package capability extracts structured capability claims from architecture documents.
//
// Parsing strategy — three document types each have distinct patterns:
//
//  1. Capability boundary docs (platform-capability-boundaries.md)
//     Detects ownership matrix table rows:
//       | <Capability name> | ✓ | ✗ | ✗ |   → owner = Nexus
//       | <Capability name> | ✗ | ✓ | ✗ |   → owner = Atlas
//       | <Capability name> | ✗ | ✗ | ✓ |   → owner = Forge
//
//  2. ADR documents (ADR-NNN-*.md)
//     Detects "## Decision" section lines:
//       "<Service> owns <capability>"
//       "<Service> is the <adjective> <capability>"
//
//  3. Service specification documents (*-specification.md)
//     Detects "## What <Service> Owns" section bullet lines.
//
// All parsing is line-by-line — no external dependencies, no regex engine.
// Returns zero claims if no patterns match. Never returns an error for
// unrecognised content — unknown documents are silently skipped.
package capability

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harshmaury/Atlas/internal/store"
)

// ── CONSTANTS ────────────────────────────────────────────────────────────────

// domainOwners maps known service names to their capability domain.
var domainOwners = map[string]string{
	"nexus": "Control",
	"atlas": "Knowledge",
	"forge": "Execution",
}

// ownerColumns maps column position (1-based) in the capability matrix
// to the owning service name.
var ownerColumns = map[int]string{
	1: "nexus",
	2: "atlas",
	3: "forge",
}

// ── PARSER ───────────────────────────────────────────────────────────────────

// ParseDocument extracts capability claims from a single architecture document.
// Returns an empty slice if no claims are found — never returns an error for
// unrecognised content.
func ParseDocument(docPath string, projectID string) ([]*store.Capability, error) {
	docType := classifyDoc(docPath)
	if docType == "" {
		return nil, nil
	}

	f, err := os.Open(docPath)
	if err != nil {
		return nil, nil // unreadable — skip silently
	}
	defer f.Close()

	switch docType {
	case "capability":
		return parseCapabilityBoundaryDoc(f, docPath, projectID), nil
	case "adr":
		return parseADR(f, docPath, projectID), nil
	case "spec":
		return parseSpecification(f, docPath, projectID), nil
	}
	return nil, nil
}

// ── CAPABILITY BOUNDARY DOC ───────────────────────────────────────────────────

// parseCapabilityBoundaryDoc parses the ownership matrix table.
// Looks for rows of the form:
//   | Capability name | ✓ | ✗ | ✗ |
// Column 1 = Nexus, Column 2 = Atlas, Column 3 = Forge.
func parseCapabilityBoundaryDoc(f *os.File, docPath, projectID string) []*store.Capability {
	var caps []*store.Capability
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Only process table rows.
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}

		cols := splitTableRow(line)
		if len(cols) < 4 {
			continue
		}

		capName := normaliseCapabilityName(cols[0])
		if capName == "" || isTableHeader(capName) {
			continue
		}

		// Check columns 1–3 for a checkmark.
		for colIdx := 1; colIdx <= 3 && colIdx < len(cols); colIdx++ {
			if strings.Contains(cols[colIdx], "✓") {
				owner, ok := ownerColumns[colIdx]
				if !ok {
					continue
				}
				caps = append(caps, &store.Capability{
					ProjectID: projectID,
					Owner:     owner,
					Domain:    domainOwners[owner],
					Name:      capName,
					DocPath:   docPath,
					DocType:   "capability",
				})
			}
		}
	}
	return caps
}

// ── ADR PARSER ────────────────────────────────────────────────────────────────

// parseADR extracts capability claims from the ## Decision section of an ADR.
// Detects lines like:
//   "Nexus is the authoritative project registry."
//   "Nexus owns filesystem observation for the entire platform."
//   "Atlas is the knowledge domain."
func parseADR(f *os.File, docPath, projectID string) []*store.Capability {
	var caps []*store.Capability
	scanner := bufio.NewScanner(f)
	inDecision := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Track section boundaries.
		if strings.HasPrefix(line, "## ") {
			inDecision = strings.EqualFold(strings.TrimPrefix(line, "## "), "decision")
			continue
		}
		if !inDecision || line == "" {
			continue
		}

		owner, capName := extractOwnershipClaim(line)
		if owner == "" || capName == "" {
			continue
		}

		caps = append(caps, &store.Capability{
			ProjectID: projectID,
			Owner:     owner,
			Domain:    domainOwners[owner],
			Name:      capName,
			DocPath:   docPath,
			DocType:   "adr",
		})
	}
	return caps
}

// ── SPECIFICATION PARSER ──────────────────────────────────────────────────────

// parseSpecification extracts capability claims from "## What X Owns" sections.
// Each bullet point under the section becomes one capability claim.
func parseSpecification(f *os.File, docPath, projectID string) []*store.Capability {
	var caps []*store.Capability
	scanner := bufio.NewScanner(f)

	var currentOwner string
	inOwnsSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Detect "## What <Service> Owns" section header.
		if strings.HasPrefix(line, "## What ") && strings.HasSuffix(strings.ToLower(line), "owns") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				svc := strings.ToLower(parts[2])
				if _, ok := domainOwners[svc]; ok {
					currentOwner = svc
					inOwnsSection = true
					continue
				}
			}
		}

		// Any other ## heading ends the owns section.
		if strings.HasPrefix(line, "## ") && inOwnsSection {
			inOwnsSection = false
			currentOwner = ""
			continue
		}

		if !inOwnsSection || currentOwner == "" {
			continue
		}

		// Bullet points are capability claims.
		capName := extractBulletItem(line)
		if capName == "" {
			continue
		}

		caps = append(caps, &store.Capability{
			ProjectID: projectID,
			Owner:     currentOwner,
			Domain:    domainOwners[currentOwner],
			Name:      capName,
			DocPath:   docPath,
			DocType:   "spec",
		})
	}
	return caps
}

// ── HELPERS ──────────────────────────────────────────────────────────────────

// classifyDoc returns the document type relevant for capability parsing,
// or "" if the document should not be parsed.
func classifyDoc(docPath string) string {
	name := strings.ToLower(filepath.Base(docPath))
	dir  := strings.ToLower(filepath.Base(filepath.Dir(docPath)))

	if dir == "decisions" && strings.HasPrefix(name, "adr-") {
		return "adr"
	}
	if strings.Contains(name, "capability") || strings.Contains(name, "boundary") {
		return "capability"
	}
	if strings.Contains(name, "-specification") || strings.Contains(name, "-spec.") {
		return "spec"
	}
	return ""
}

// splitTableRow splits a Markdown table row into trimmed column values.
// "|  Foo  |  ✓  |  ✗  |" → ["Foo", "✓", "✗"]
func splitTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cols := make([]string, 0, len(parts))
	for _, p := range parts {
		cols = append(cols, strings.TrimSpace(p))
	}
	return cols
}

// normaliseCapabilityName converts a raw capability name to a slug.
// "Project registry (ADR-001)" → "project-registry"
func normaliseCapabilityName(raw string) string {
	// Strip parenthetical notes.
	if idx := strings.Index(raw, "("); idx >= 0 {
		raw = raw[:idx]
	}
	raw = strings.TrimSpace(raw)
	raw = strings.ToLower(raw)
	raw = strings.ReplaceAll(raw, " ", "-")
	raw = strings.ReplaceAll(raw, "/", "-")
	// Remove trailing dashes.
	raw = strings.Trim(raw, "-")
	return raw
}

// isTableHeader returns true for separator or header rows.
func isTableHeader(name string) bool {
	return strings.ContainsAny(name, ":—-") &&
		!strings.ContainsAny(name, "abcdefghijklmnopqrstuvwxyz")
}

// extractOwnershipClaim parses a line like:
//   "Nexus is the authoritative project registry."
//   "Nexus owns filesystem observation for the entire platform."
// Returns (owner, capabilityName) or ("", "").
func extractOwnershipClaim(line string) (owner, capName string) {
	lower := strings.ToLower(line)

	for svc := range domainOwners {
		prefix := svc + " "
		if !strings.HasPrefix(lower, prefix) {
			continue
		}

		rest := line[len(prefix):]

		// "owns <capability>"
		if _, ok := cutPrefix(strings.ToLower(rest), "owns "); ok {
			cap := extractCapPhrase(rest[len("owns "):])
			if cap != "" {
				return svc, cap
			}
		}

		// "is the <adjective?> <capability>"
		if _, ok := cutPrefix(strings.ToLower(rest), "is the "); ok {
			cap := extractCapPhrase(rest[len("is the "):])
			if cap != "" {
				return svc, cap
			}
		}
	}
	return "", ""
}

// extractCapPhrase takes the remainder of a claim line and returns
// the first meaningful noun phrase (up to a period, comma, or em-dash).
func extractCapPhrase(s string) string {
	// Cut at sentence-ending punctuation.
	for _, sep := range []string{".", ",", " —", " for ", " and "} {
		if idx := strings.Index(strings.ToLower(s), sep); idx > 0 {
			s = s[:idx]
			break
		}
	}
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.Trim(s, "-.")
	if len(s) < 3 {
		return ""
	}
	return s
}

// extractBulletItem returns the text of a Markdown bullet point, or "".
func extractBulletItem(line string) string {
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, prefix) {
			item := strings.TrimSpace(line[len(prefix):])
			return normaliseCapabilityName(item)
		}
	}
	return ""
}

// cutPrefix is strings.CutPrefix for Go versions before 1.20.
func cutPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):], true
	}
	return s, false
}
