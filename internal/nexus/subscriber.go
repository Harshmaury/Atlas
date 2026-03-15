// @atlas-project: atlas
// @atlas-path: internal/nexus/subscriber.go
// Subscriber connects to the Nexus event bus over HTTP long-poll
// and delivers workspace change events to Atlas index handlers.
//
// ADR-002: Nexus owns filesystem observation. Atlas subscribes to
// workspace events via the Nexus event bus — it never runs a watcher.
//
// Transport: Atlas polls GET /events?limit=1&since=<last_id> on a
// short interval. This is simpler than a persistent WebSocket and
// consistent with the existing HTTP/JSON API pattern (ADR-003).
// A push-based subscription (SSE or WebSocket) can replace this
// in a future phase without changing the subscriber interface.
//
// Topic constants are imported from Nexus eventbus — never redefined.
package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	nexusevents "github.com/Harshmaury/Nexus/pkg/events"
)

const (
	pollInterval   = 3 * time.Second
	pollEventLimit = 50
)

// WorkspaceEvent is a workspace change event received from Nexus.
type WorkspaceEvent struct {
	ID        int64
	Topic     nexusevents.Topic
	Payload   json.RawMessage
	CreatedAt time.Time
}

// EventHandler is called for each workspace event received.
type EventHandler func(event WorkspaceEvent)

// Subscriber polls the Nexus events API for workspace change events
// and delivers them to registered handlers.
type Subscriber struct {
	client   *Client
	handlers map[nexusevents.Topic][]EventHandler
	lastID   int64
}

// NewSubscriber creates a Subscriber.
func NewSubscriber(client *Client) *Subscriber {
	return &Subscriber{
		client:   client,
		handlers: make(map[nexusevents.Topic][]EventHandler),
	}
}

// Subscribe registers a handler for a specific workspace topic.
// Uses Nexus eventbus topic constants — ADR-002 consumer rule.
func (s *Subscriber) Subscribe(topic nexusevents.Topic, handler EventHandler) {
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

// poll fetches recent events from Nexus and dispatches workspace events.
func (s *Subscriber) poll(ctx context.Context) {
	url := fmt.Sprintf("%s/events?limit=%d", s.client.baseURL, pollEventLimit)
	if s.lastID > 0 {
		url += "&since=" + strconv.FormatInt(s.lastID, 10)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}

	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return // Nexus may be temporarily unavailable — retry next tick
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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

		// Only dispatch workspace topics — ADR-002.
		topic := nexusevents.Topic(e.Type)
		switch topic {
		case nexusevents.TopicWorkspaceFileCreated,
			nexusevents.TopicWorkspaceFileModified,
			nexusevents.TopicWorkspaceFileDeleted,
			nexusevents.TopicWorkspaceUpdated,
			nexusevents.TopicWorkspaceProjectDetected:
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
