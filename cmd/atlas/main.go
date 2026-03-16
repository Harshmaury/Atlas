// @atlas-project: atlas
// @atlas-path: cmd/atlas/main.go
// ADR-008: ATLAS_SERVICE_TOKEN env var read at startup.
//   Set on nexus.Client (outbound) and api.ServerConfig (inbound from Forge).
//
// AT-Fix-01: path containment check in reindexOnEvent uses filepath.Rel
//   instead of raw string prefix. Eliminates false matches where a project
//   path is a string prefix of an unrelated sibling path.
//   e.g. project=/workspace/atlas no longer matches /workspace/atlas-old/file.
//
// AT-Fix-02: reindexOnEvent no longer calls capIndexer.IndexAll() and
//   graphBuilder.BuildAll() on every file event. Both are full workspace
//   scans — O(projects × docs) — triggered on every keystroke when editors
//   autosave. Replaced with targeted per-document re-index:
//   capIndexer.IndexDocument(path, projectID) re-indexes only the changed
//   file's capability claims. graphBuilder.BuildAll() is deferred to the
//   TopicWorkspaceUpdated batch signal which fires once per debounce window
//   rather than once per file event.
//
// atlas is the Atlas knowledge service daemon.
// It indexes the workspace, subscribes to Nexus workspace events,
// and serves the Atlas HTTP API on 127.0.0.1:8081.
//
// Startup sequence:
//  1. Config (env vars)
//  2. Store (SQLite index database)
//  3. Nexus client (project registry query — ADR-001)
//  4. Discovery (workspace scan)
//  5. Indexers (source + document)
//  6. Context generator
//  7. Capability indexer     ← Phase 2
//  8. Graph builder          ← Phase 2
//  9. Query runner           ← Phase 2
// 10. HTTP API server (:8081 — ADR-003)
// 11. Event subscriber (Nexus workspace events — ADR-002)
// 12. Initial index run (blocking — completes before serving)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/Harshmaury/Atlas/internal/api"
	atlascontext "github.com/Harshmaury/Atlas/internal/context"
	"github.com/Harshmaury/Atlas/internal/capability"
	"github.com/Harshmaury/Atlas/internal/config"
	"github.com/Harshmaury/Atlas/internal/discovery"
	"github.com/Harshmaury/Atlas/internal/graph"
	"github.com/Harshmaury/Atlas/internal/indexer"
	nexusclient "github.com/Harshmaury/Atlas/internal/nexus"
	"github.com/Harshmaury/Atlas/internal/store"
	nexusevents "github.com/Harshmaury/Nexus/pkg/events"
)

const atlasVersion = "0.2.0"

func main() {
	logger := log.New(os.Stdout, "[atlas] ", log.LstdFlags)
	logger.Printf("Atlas v%s starting", atlasVersion)
	if err := run(logger); err != nil {
		logger.Fatalf("fatal: %v", err)
	}
	logger.Println("Atlas stopped cleanly")
}

func run(logger *log.Logger) error {
	// ── 1. CONFIG ────────────────────────────────────────────────────────────
	httpAddr      := config.EnvOrDefault("ATLAS_HTTP_ADDR", config.DefaultHTTPAddr)
	workspaceRoot := config.ExpandHome(config.EnvOrDefault("ATLAS_WORKSPACE", config.DefaultWorkspace))
	nexusAddr     := config.EnvOrDefault("NEXUS_HTTP_ADDR", config.DefaultNexusAddr)
	dbPath        := config.ExpandHome(config.EnvOrDefault("ATLAS_DB_PATH", config.DefaultDBPath))
	// ADR-008: outbound token for Nexus calls + inbound token expected from Forge.
	serviceToken  := config.EnvOrDefault("ATLAS_SERVICE_TOKEN", "")
	if serviceToken == "" {
		logger.Println("WARNING: ATLAS_SERVICE_TOKEN not set — inter-service auth disabled")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	// ── 2. STORE ─────────────────────────────────────────────────────────────
	logger.Printf("opening index store: %s", dbPath)
	s, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	// ── 3. NEXUS CLIENT ──────────────────────────────────────────────────────
	nexus := nexusclient.New(nexusAddr).WithServiceToken(serviceToken)

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if err := nexus.Ping(ctx); err != nil {
		logger.Printf("WARNING: Nexus not reachable at %s — will retry: %v", nexusAddr, err)
	} else {
		logger.Printf("Nexus connected at %s", nexusAddr)
	}

	// ── 4. DISCOVERY ─────────────────────────────────────────────────────────
	scanner := discovery.NewScanner(workspaceRoot)

	// ── 5. INDEXERS ──────────────────────────────────────────────────────────
	srcIndexer := indexer.NewSourceIndexer(s)
	docIndexer := indexer.NewDocumentIndexer(s)

	// ── 6. CONTEXT GENERATOR ─────────────────────────────────────────────────
	generator := atlascontext.New(s, workspaceRoot)

	// ── 7. CAPABILITY INDEXER (Phase 2) ──────────────────────────────────────
	capIndexer := capability.NewIndexer(s, logger)

	// ── 8. GRAPH BUILDER (Phase 2) ───────────────────────────────────────────
	graphBuilder := graph.NewBuilder(s, logger)

	// ── 9. QUERY RUNNER (Phase 2) ────────────────────────────────────────────
	queryRunner := graph.NewQueryRunner(s)

	// ── 10. HTTP API ──────────────────────────────────────────────────────────
	apiServer := api.NewServer(api.ServerConfig{
		Addr:         httpAddr,
		Store:        s,
		Generator:    generator,
		QueryRunner:  queryRunner,
		Logger:       logger,
		ServiceToken: serviceToken, // ADR-008: token expected from Forge
	})

	// ── 11. EVENT SUBSCRIBER (ADR-002) ────────────────────────────────────────
	subscriber := nexusclient.NewSubscriber(nexus)

	// reindexOnEvent is called for every workspace file event (created/modified/deleted).
	// AT-Fix-01: containment check uses filepath.Rel — eliminates false matches on
	//            sibling paths that share a string prefix (e.g. atlas vs atlas-old).
	// AT-Fix-02: only the affected project's source + documents are re-indexed.
	//            capability claims are re-indexed for the specific changed file only.
	//            graphBuilder.BuildAll() is NOT called here — it runs on the debounced
	//            TopicWorkspaceUpdated signal (one call per quiet window, not per file).
	reindexOnEvent := func(event nexusclient.WorkspaceEvent) {
		var payload nexusevents.WorkspaceFilePayload
		if err := unmarshalPayload(event.Payload, &payload); err != nil {
			return
		}
		if payload.Path == "" {
			return
		}
		projects, err := s.GetAllProjects()
		if err != nil {
			return
		}
		for _, p := range projects {
			if p.Path == "" {
				continue
			}
			// filepath.Rel returns a path without ".." only when payload.Path
			// is inside p.Path — the correct containment check.
			rel, err := filepath.Rel(p.Path, payload.Path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			// Re-index source and documents for the affected project.
			if _, err := srcIndexer.IndexProject(p); err != nil {
				logger.Printf("WARNING: source re-index %s: %v", p.ID, err)
			}
			if _, err := docIndexer.IndexProject(p); err != nil {
				logger.Printf("WARNING: doc re-index %s: %v", p.ID, err)
			}
			// Re-index capability claims for this specific file only (not IndexAll).
			if _, err := capIndexer.IndexDocument(payload.Path, p.ID); err != nil {
				logger.Printf("WARNING: capability re-index %s: %v", payload.Path, err)
			}
			logger.Printf("re-indexed %s (triggered by %s)", p.ID, payload.Name)
			return
		}
	}

	subscriber.Subscribe(nexusevents.TopicWorkspaceFileCreated,  reindexOnEvent)
	subscriber.Subscribe(nexusevents.TopicWorkspaceFileModified, reindexOnEvent)
	subscriber.Subscribe(nexusevents.TopicWorkspaceFileDeleted,  reindexOnEvent)

	// TopicWorkspaceUpdated fires once per debounce window after a burst of
	// file events settles. Running BuildAll here (rather than per-file) means
	// graph edges are rebuilt at most once per quiet period — not once per save.
	subscriber.Subscribe(nexusevents.TopicWorkspaceUpdated,
		func(event nexusclient.WorkspaceEvent) {
			if _, err := graphBuilder.BuildAll(); err != nil {
				logger.Printf("WARNING: graph rebuild on workspace update: %v", err)
			}
		},
	)

	subscriber.Subscribe(nexusevents.TopicWorkspaceProjectDetected,
		func(event nexusclient.WorkspaceEvent) {
			logger.Printf("new project detected — running full index")
			runFullIndex(ctx, logger, nexus, scanner, srcIndexer, docIndexer,
				capIndexer, graphBuilder, workspaceRoot, s)
		},
	)

	// ── 12. INITIAL INDEX ─────────────────────────────────────────────────────
	logger.Printf("running initial workspace index (root=%s)", workspaceRoot)
	runFullIndex(ctx, logger, nexus, scanner, srcIndexer, docIndexer,
		capIndexer, graphBuilder, workspaceRoot, s)

	files, _  := s.CountFiles()
	docs, _   := s.CountDocuments()
	caps, _   := s.CountCapabilities()
	logger.Printf("index complete — %d files, %d documents, %d capabilities", files, docs, caps)

	// ── START GOROUTINES ──────────────────────────────────────────────────────
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := apiServer.Run(ctx); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("api server: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Printf("event subscriber started (polling Nexus every 3s)")
		if err := subscriber.Run(ctx); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("subscriber: %w", err)
		}
	}()

	logger.Printf("✓ Atlas ready — http=%s workspace=%s db=%s",
		httpAddr, workspaceRoot, dbPath)

	// ── WAIT FOR SHUTDOWN ─────────────────────────────────────────────────────
	select {
	case sig := <-sigCh:
		logger.Printf("received %s — shutting down", sig)
	case err := <-errCh:
		logger.Printf("component error: %v — shutting down", err)
	}

	cancel()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	<-done

	logger.Println("all components stopped cleanly")
	return nil
}

// runFullIndex discovers projects and indexes source, documents, capabilities, and graph.
func runFullIndex(
	ctx context.Context,
	logger *log.Logger,
	nexus *nexusclient.Client,
	scanner *discovery.Scanner,
	srcIdx *indexer.SourceIndexer,
	docIdx *indexer.DocumentIndexer,
	capIdx *capability.Indexer,
	graphBld *graph.Builder,
	workspaceRoot string,
	s store.Storer,
) {
	nexusProjects, err := nexus.GetProjects(ctx)
	if err != nil {
		logger.Printf("WARNING: cannot fetch Nexus projects: %v", err)
		nexusProjects = nil
	}

	scanned, err := scanner.ScanWorkspace()
	if err != nil {
		logger.Printf("WARNING: workspace scan error: %v", err)
	}

	projects := discovery.MergeWithNexus(scanned, nexusProjects)

	s.UpsertProject(&store.Project{ //nolint:errcheck
		ID:     "platform",
		Name:   "Platform Architecture",
		Path:   workspaceRoot,
		Source: "detected",
	})

	for _, p := range projects {
		s.UpsertProject(p) //nolint:errcheck

		if _, err := srcIdx.IndexProject(p); err != nil {
			logger.Printf("WARNING: source index %s: %v", p.ID, err)
		}
		if _, err := docIdx.IndexProject(p); err != nil {
			logger.Printf("WARNING: doc index %s: %v", p.ID, err)
		}
	}

	archDir := workspaceRoot + "/architecture"
	docIdx.IndexWorkspaceArchitecture(archDir) //nolint:errcheck

	// Phase 2 — capability and graph index runs after all docs are indexed.
	if result, err := capIdx.IndexAll(); err != nil {
		logger.Printf("WARNING: capability index: %v", err)
	} else {
		logger.Printf("capability index — %d claims from %d docs",
			result.ClaimsStored, result.DocsIndexed)
	}

	if result, err := graphBld.BuildAll(); err != nil {
		logger.Printf("WARNING: graph build: %v", err)
	} else {
		logger.Printf("graph build — %d nexus.yaml + %d import + %d ADR ref edges",
			result.EdgesFromNexusYAML, result.EdgesFromImports, result.EdgesFromADRRefs)
	}
}

func unmarshalPayload(raw []byte, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("empty payload")
	}
	return json.Unmarshal(raw, v)
}
