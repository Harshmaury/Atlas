module github.com/Harshmaury/Atlas

go 1.23.0

require (
	// SQLite — index store and FTS5 full-text search
	// Same driver as Nexus — no new toolchain dependency
	github.com/mattn/go-sqlite3 v1.14.34

	// YAML — parse .nexus.yaml manifests and architecture documents
	gopkg.in/yaml.v3 v3.0.1

	// Nexus eventbus — import topic constants only (ADR-002)
	// Atlas subscribes to workspace events; never calls Publish
	github.com/Harshmaury/Nexus v0.0.0
)

// Replace directive points to local Nexus for topic constant imports
// Update this to a tagged release once Nexus publishes one
replace github.com/Harshmaury/Nexus => ../nexus
