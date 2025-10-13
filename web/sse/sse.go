// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package sse provides a server implementation for Server-Sent Events (SSE).
package sse

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"go.astrophena.name/base/web"
)

const clientChanBuf = 16

// Streamer manages a pool of connected SSE clients and broadcasts messages to them.
// A Streamer must not be copied after first use.
type Streamer struct {
	mu      sync.RWMutex
	clients map[chan string]struct{}
}

// NewStreamer creates a new, ready-to-use Streamer.
func NewStreamer() *Streamer {
	return &Streamer{
		clients: make(map[chan string]struct{}),
	}
}

// ErrStreamingUnsupported is returned when SSE is unsupported for the HTTP
// connection.
var ErrStreamingUnsupported = errors.New("streaming unsupported")

// ServeHTTP implements the [http.Handler] interface.
func (s *Streamer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		web.RespondError(w, r, ErrStreamingUnsupported)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	clientChan := make(chan string, clientChanBuf)

	s.mu.Lock()
	s.clients[clientChan] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, clientChan)
		s.mu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			// Client has disconnected.
			return
		case msg := <-clientChan:
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

// Send broadcasts a plain text message to all connected clients.
// The event name will be "message".
func (s *Streamer) Send(data string) {
	s.SendEvent("message", data)
}

// SendEvent broadcasts a message with a custom event name to all connected clients.
func (s *Streamer) SendEvent(event, data string) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "event: %s\n", event)
	fmt.Fprintf(&buf, "data: %s\n\n", data)
	msg := buf.String()

	s.broadcast(msg)
}

// SendJSON marshals a Go value to JSON and broadcasts it as an event to all
// connected clients.
func (s *Streamer) SendJSON(event string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("sse: failed to marshal JSON: %w", err)
	}
	s.SendEvent(event, string(data))
	return nil
}

// broadcast sends a pre-formatted message to all clients.
// It uses a non-blocking send to prevent a slow client from blocking all others.
func (s *Streamer) broadcast(msg string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for client := range s.clients {
		select {
		case client <- msg:
			// Message sent successfully.
		default:
			// Client's channel buffer is full. This indicates a slow client.
			// We drop the message for this client to avoid blocking the broadcast.
		}
	}
}

// ClientCount returns the number of currently connected clients.
func (s *Streamer) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}
