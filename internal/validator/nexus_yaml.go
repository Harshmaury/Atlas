// @atlas-project: atlas
// @atlas-path: internal/validator/nexus_yaml.go
// Package validator parses and validates nexus.yaml project descriptors.
//
// ADR-009: nexus.yaml is the mandatory descriptor for full Atlas graph
// membership. Projects without a valid descriptor are indexed as
// status=unverified — they appear in API responses but are excluded
// from capability graphs and conflict detection.
//
// Validation is intentionally lenient: unknown fields are ignored,
// parse errors produce unverified status rather than hard failures.
// This keeps Atlas resilient to projects at different stages of adoption.
package validator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ── CONSTANTS ────────────────────────────────────────────────────────────────

const NexusYAMLFile = "nexus.yaml"

// ValidTypes is the set of allowed project type values (ADR-009).
var ValidTypes = map[string]bool{
	"platform-daemon": true,
	"web-api":         true,
	"worker":          true,
	"cli":             true,
	"database":        true,
	"message-broker":  true,
	"gateway":         true,
	"library":         true,
	"automation":      true,
	"ml-service":      true,
	"governance":      true,
	"tool":            true,
}

// semverRE matches a basic semver string e.g. 1.0.0 or 2.3.1-alpha.
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// ── DESCRIPTOR ───────────────────────────────────────────────────────────────

// Descriptor is the parsed content of a nexus.yaml file.
// Required fields: Name, ID, Type, Language, Version.
// Optional fields: Keywords, Capabilities, DependsOn, Runtime.
type Descriptor struct {
	Name         string   `yaml:"name"`
	ID           string   `yaml:"id"`
	Type         string   `yaml:"type"`
	Language     string   `yaml:"language"`
	Version      string   `yaml:"version"`
	Keywords     []string `yaml:"keywords"`
	Capabilities []string `yaml:"capabilities"`
	DependsOn    []string `yaml:"depends_on"`
	Runtime      Runtime  `yaml:"runtime"`
}

// Runtime describes how the project is executed.
type Runtime struct {
	Provider string `yaml:"provider"` // process | docker | k8s
	Port     int    `yaml:"port"`
}

// ── RESULT ───────────────────────────────────────────────────────────────────

// Result is the outcome of validating a nexus.yaml file.
type Result struct {
	Descriptor *Descriptor // nil if parse failed
	Valid       bool        // true only if all required fields pass
	Errors      []string    // human-readable validation errors
}

// StatusString returns "verified" or "unverified" for use in API responses.
func (r *Result) StatusString() string {
	if r.Valid {
		return "verified"
	}
	return "unverified"
}

// ── VALIDATOR ────────────────────────────────────────────────────────────────

// ValidateDir reads nexus.yaml from projectPath and validates it.
// Returns a Result with Valid=false and a reason if the file is absent
// or invalid. Never returns a hard error — missing/invalid descriptors
// are expected for legacy projects.
func ValidateDir(projectPath string) *Result {
	data, err := os.ReadFile(filepath.Join(projectPath, NexusYAMLFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Result{Errors: []string{"nexus.yaml not found"}}
		}
		return &Result{Errors: []string{fmt.Sprintf("read nexus.yaml: %s", err)}}
	}
	return ValidateBytes(data)
}

// ValidateBytes parses and validates raw nexus.yaml content.
// Used directly in tests and by ValidateDir.
func ValidateBytes(data []byte) *Result {
	var d Descriptor
	if err := yaml.Unmarshal(data, &d); err != nil {
		return &Result{Errors: []string{fmt.Sprintf("parse nexus.yaml: %s", err)}}
	}
	return validate(&d)
}

// validate runs all field-level checks against a parsed descriptor.
func validate(d *Descriptor) *Result {
	var errs []string

	if strings.TrimSpace(d.Name) == "" {
		errs = append(errs, "name is required")
	}

	if strings.TrimSpace(d.ID) == "" {
		errs = append(errs, "id is required")
	} else if strings.Contains(d.ID, " ") {
		errs = append(errs, "id must not contain spaces")
	}

	if strings.TrimSpace(d.Type) == "" {
		errs = append(errs, "type is required")
	} else if !ValidTypes[d.Type] {
		errs = append(errs, fmt.Sprintf("type %q is not valid", d.Type))
	}

	if strings.TrimSpace(d.Language) == "" {
		errs = append(errs, "language is required")
	}

	if strings.TrimSpace(d.Version) == "" {
		errs = append(errs, "version is required")
	} else if !semverRE.MatchString(d.Version) {
		errs = append(errs, fmt.Sprintf("version %q must start with semver (N.N.N)", d.Version))
	}

	if len(errs) > 0 {
		return &Result{Descriptor: d, Errors: errs}
	}
	return &Result{Descriptor: d, Valid: true}
}
