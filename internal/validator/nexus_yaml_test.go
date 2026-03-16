// @atlas-project: atlas
// @atlas-path: internal/validator/nexus_yaml_test.go
package validator

import (
	"strings"
	"testing"
)

func TestValidateBytes(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValid bool
		wantErr   string // substring expected in errors, empty = no check
	}{
		{
			name: "valid full descriptor",
			input: `
name: my-project
id: my-project
type: web-api
language: go
version: 1.0.0
keywords:
  - my-project
capabilities:
  - rest-api
depends_on:
  - postgres
runtime:
  provider: process
  port: 8090
`,
			wantValid: true,
		},
		{
			name: "valid minimal descriptor",
			input: `
name: nexus
id: nexus
type: platform-daemon
language: go
version: 0.1.0
`,
			wantValid: true,
		},
		{
			name:      "missing name",
			input:     "id: x\ntype: cli\nlanguage: go\nversion: 1.0.0\n",
			wantValid: false,
			wantErr:   "name is required",
		},
		{
			name:      "missing id",
			input:     "name: x\ntype: cli\nlanguage: go\nversion: 1.0.0\n",
			wantValid: false,
			wantErr:   "id is required",
		},
		{
			name:      "id with spaces",
			input:     "name: x\nid: my project\ntype: cli\nlanguage: go\nversion: 1.0.0\n",
			wantValid: false,
			wantErr:   "id must not contain spaces",
		},
		{
			name:      "invalid type",
			input:     "name: x\nid: x\ntype: server\nlanguage: go\nversion: 1.0.0\n",
			wantValid: false,
			wantErr:   "is not valid",
		},
		{
			name:      "missing language",
			input:     "name: x\nid: x\ntype: cli\nversion: 1.0.0\n",
			wantValid: false,
			wantErr:   "language is required",
		},
		{
			name:      "missing version",
			input:     "name: x\nid: x\ntype: cli\nlanguage: go\n",
			wantValid: false,
			wantErr:   "version is required",
		},
		{
			name:      "invalid semver",
			input:     "name: x\nid: x\ntype: cli\nlanguage: go\nversion: latest\n",
			wantValid: false,
			wantErr:   "must start with semver",
		},
		{
			name:      "semver with prerelease suffix",
			input:     "name: x\nid: x\ntype: cli\nlanguage: go\nversion: 1.0.0-alpha\n",
			wantValid: true,
		},
		{
			name:      "invalid yaml",
			input:     "name: [unclosed",
			wantValid: false,
			wantErr:   "parse nexus.yaml",
		},
		{
			name:      "unknown fields ignored",
			input:     "name: x\nid: x\ntype: cli\nlanguage: go\nversion: 1.0.0\nfuture_field: value\n",
			wantValid: true,
		},
		{
			name:      "empty input",
			input:     "",
			wantValid: false,
			wantErr:   "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ValidateBytes([]byte(tt.input))

			if r.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v — errors: %v", r.Valid, tt.wantValid, r.Errors)
			}

			if tt.wantErr != "" {
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e, tt.wantErr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, r.Errors)
				}
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	if (&Result{Valid: true}).StatusString() != "verified" {
		t.Error("expected verified")
	}
	if (&Result{Valid: false}).StatusString() != "unverified" {
		t.Error("expected unverified")
	}
}
