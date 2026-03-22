// @atlas-project: atlas
// @atlas-path: internal/nexus/subscriber.go
// ADR-008 fix: poll() now uses client.get() so X-Service-Token is sent.
//   Previously poll() built its own http.NewRequestWithContext directly,
//   bypassing the get() helper that injects the token.
//
// Subscriber connects to the Nexus event bus over HTTP long-poll
// and delivers workspace change events to Atlas index handlers.
//
// ADR-002: Nexus owns filesystem observation. Atlas subscribes to
// workspace events via the Nexus event bus — it never runs a watcher.
//
// ADR-045: topic constants and payload types now imported from Canon.
// No longer imports github.com/Harshmaury/Nexus/pkg/events.
package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	canonevents "github.com/Harshmaury/Canon/events"
)

const (
	pollInterval   = 3 * time.Second
	pollEventLimit = 50
)

// WorkspaceEvent is a workspace change event received from Nexus.
type WorkspaceEvent struct {
	ID        int64
	Topic     canonevents.TopicType
	Payload   json.RawMessage
	CreatedAt time.Time
}

// EventHandler is called for each workspace event received.
type EventHandler func(event WorkspaceEvent)

// Subscriber polls the Nexus events API for workspace change events
// and delivers them to registered handlers.
type Subscriber struct {
	client   *Client
	handlers map[canonevents.TopicType][]EventHandler
	lastID   int64
}

// NewSubscriber creates a Subscriber.
func NewSubscriber(client *Client) *Subscriber {
	return &Subscriber{
		client:   client,
		handlers: make(map[canonevents.TopicType][]EventHandler),
	}
}

// Subscribe registers a handler for a specific workspace topic.
// Uses Canon topic constants — ADR-002, ADR-045.
func (s *Subscriber) Subscribe(topic canonevents.TopicType, handler EventHandler) {
	s.handlers[topic] = append(s.handlers[topic], handler)
}

// Run starts the polling loop and blocks until ctx is cancelled.
func (s *Subscriber) Run(ctx context.Context) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll fetches events from Nexus since the last seen ID and dispatches workspace events.
func (s *Subscriber) poll(ctx context.Context) {
	var path string
	if s.lastID > 0 {
		path = fmt.Sprintf("/events?since=%d&limit=%d", s.lastID, pollEventLimit)
	} else {
		path = fmt.Sprintf("/events?limit=%d", pollEventLimit)
	}

	resp, err := s.client.get(ctx, path)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[atlas/subscriber] WARNING: Nexus poll returned HTTP %d — will retry next tick\n", resp.StatusCode)
		return
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data []struct {
			ID        int64           `json:"id"`
			Type      string          `json:"type"`
			ServiceID string          `json:"service_id"`
			Payload   json.RawMessage `json:"payload"`
			CreatedAt time.Time       `json:"created_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return
	}
	if !envelope.OK {
		return
	}

	for _, e := range envelope.Data {
		if e.ID > s.lastID {
			s.lastID = e.ID
		}

		topic := canonevents.TopicType(e.Type)
		switch topic {
		case canonevents.TopicWorkspaceFileCreated,
			canonevents.TopicWorkspaceFileModified,
			canonevents.TopicWorkspaceFileDeleted,
			canonevents.TopicWorkspaceUpdated,
			canonevents.TopicWorkspaceProjectDetected:
			event := WorkspaceEvent{
				ID:        e.ID,
				Topic:     topic,
				Payload:   e.Payload,
				CreatedAt: e.CreatedAt,
			}
			for _, h := range s.handlers[topic] {
				h(event)
			}
		}
	}
}

