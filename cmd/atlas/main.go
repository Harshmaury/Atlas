// @atlas-project: atlas
// @atlas-path: cmd/atlas/main.go
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
//  7. HTTP API server (:8081 — ADR-003)
//  8. Event subscriber (Nexus workspace events — ADR-002)
//  9. Initial index run (blocking — completes before serving)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/Harshmaury/Atlas/internal/api"
	atlascontext "github.com/Harshmaury/Atlas/internal/context"
	"github.com/Harshmaury/Atlas/internal/config"
	"github.com/Harshmaury/Atlas/internal/discovery"
	"github.com/Harshmaury/Atlas/internal/indexer"
	nexusclient "github.com/Harshmaury/Atlas/internal/nexus"
	"github.com/Harshmaury/Atlas/internal/store"
	nexusevents "github.com/Harshmaury/Nexus/pkg/events"
)

const atlasVersion = "0.1.0"

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

	// Ensure db directory exists.
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
	nexus := nexusclient.New(nexusAddr)

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Check Nexus reachability — warn but continue if unavailable.
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

	// ── 7. HTTP API ──────────────────────────────────────────────────────────
	apiServer := api.NewServer(api.ServerConfig{
		Addr:      httpAddr,
		Store:     s,
		Generator: generator,
		Logger:    logger,
	})

	// ── 8. EVENT SUBSCRIBER (ADR-002) ────────────────────────────────────────
	subscriber := nexusclient.NewSubscriber(nexus)

	// On workspace file events — re-index the affected project.
	reindexOnEvent := func(event nexusclient.WorkspaceEvent) {
		var payload nexusevents.WorkspaceFilePayload
		if err := unmarshalPayload(event.Payload, &payload); err != nil {
			return
		}
		// Find which project owns this file and re-index it.
		projects, err := s.GetAllProjects()
		if err != nil {
			return
		}
		for _, p := range projects {
			if p.Path != "" && len(payload.Path) >= len(p.Path) &&
				payload.Path[:len(p.Path)] == p.Path {
				srcIndexer.IndexProject(p)  //nolint:errcheck — best effort
				docIndexer.IndexProject(p)  //nolint:errcheck
				logger.Printf("re-indexed %s (triggered by %s)", p.ID, payload.Name)
				return
			}
		}
	}

	subscriber.Subscribe(nexusevents.TopicWorkspaceFileCreated,  reindexOnEvent)
	subscriber.Subscribe(nexusevents.TopicWorkspaceFileModified, reindexOnEvent)
	subscriber.Subscribe(nexusevents.TopicWorkspaceFileDeleted,  reindexOnEvent)

	// On project detected — run full discovery and re-index.
	subscriber.Subscribe(nexusevents.TopicWorkspaceProjectDetected,
		func(event nexusclient.WorkspaceEvent) {
			logger.Printf("new project detected — running full index")
			runFullIndex(ctx, logger, nexus, scanner, srcIndexer, docIndexer, workspaceRoot, s)
		},
	)

	// ── 9. INITIAL INDEX ─────────────────────────────────────────────────────
	logger.Printf("running initial workspace index (root=%s)", workspaceRoot)
	runFullIndex(ctx, logger, nexus, scanner, srcIndexer, docIndexer, workspaceRoot, s)

	files, _ := s.CountFiles()
	docs, _  := s.CountDocuments()
	logger.Printf("index complete — %d files, %d documents", files, docs)

	// ── START GOROUTINES ─────────────────────────────────────────────────────
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

	// ── WAIT FOR SHUTDOWN ────────────────────────────────────────────────────
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

// runFullIndex discovers projects and indexes all source and documents.
func runFullIndex(
	ctx context.Context,
	logger *log.Logger,
	nexus *nexusclient.Client,
	scanner *discovery.Scanner,
	srcIdx *indexer.SourceIndexer,
	docIdx *indexer.DocumentIndexer,
	workspaceRoot string,
	s store.Storer,
) {
	// Fetch authoritative project list from Nexus (ADR-001).
	nexusProjects, err := nexus.GetProjects(ctx)
	if err != nil {
		logger.Printf("WARNING: cannot fetch Nexus projects: %v", err)
		nexusProjects = nil
	}

	// Scan workspace for all projects.
	scanned, err := scanner.ScanWorkspace()
	if err != nil {
		logger.Printf("WARNING: workspace scan error: %v", err)
	}

	// Merge — Nexus is authoritative.
	projects := discovery.MergeWithNexus(scanned, nexusProjects)

	// Ensure platform pseudo-project exists for workspace-level docs.
	s.UpsertProject(&store.Project{ //nolint:errcheck
		ID:     "platform",
		Name:   "Platform Architecture",
		Path:   workspaceRoot,
		Source: "detected",
	})

	for _, p := range projects {
		s.UpsertProject(p) //nolint:errcheck — best effort

		if _, err := srcIdx.IndexProject(p); err != nil {
			logger.Printf("WARNING: source index %s: %v", p.ID, err)
		}
		if _, err := docIdx.IndexProject(p); err != nil {
			logger.Printf("WARNING: doc index %s: %v", p.ID, err)
		}
	}

	// Index workspace-level architecture directory.
	archDir := workspaceRoot + "/architecture"
	docIdx.IndexWorkspaceArchitecture(archDir) //nolint:errcheck
}

// unmarshalPayload decodes a JSON event payload.
func unmarshalPayload(raw []byte, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("empty payload")
	}
	return json.Unmarshal(raw, v)
}
