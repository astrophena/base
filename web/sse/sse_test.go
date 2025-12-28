// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.astrophena.name/base/testutil"
)

func readEvent(t *testing.T, r *bufio.Reader) (event, data string) {
	t.Helper()

	// An SSE event is a block of text terminated by two newlines.
	// We read line by line until we find an empty line.
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				t.Fatalf("failed to read from stream: %v", err)
			}
			// If we hit EOF but have some data, it might be a partial event.
			// For this test, we'll treat it as the end.
			if event != "" || data != "" {
				return
			}
			t.Fatalf("stream closed unexpectedly with no event data")
		}

		line = strings.TrimSpace(line)

		// An empty line signals the end of the event.
		if line == "" {
			return
		}

		key, value, found := strings.Cut(line, ": ")
		if !found {
			t.Fatalf("malformed SSE line: %q", line)
		}

		switch key {
		case "event":
			event = value
		case "data":
			data = value
		}
	}
}

func TestStreamer_ServeHTTP_Headers(t *testing.T) {
	streamer := NewStreamer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Use a cancellable context to immediately terminate the handler.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req = req.WithContext(ctx)

	streamer.ServeHTTP(w, req)

	res := w.Result()
	testutil.AssertEqual(t, res.Header.Get("Content-Type"), "text/event-stream")
	testutil.AssertEqual(t, res.Header.Get("Cache-Control"), "no-cache")
	testutil.AssertEqual(t, res.Header.Get("Connection"), "keep-alive")
}

func TestStreamer_SingleClient(t *testing.T) {
	t.Parallel()

	streamer := NewStreamer()
	server := httptest.NewServer(streamer)
	defer server.Close()

	res, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to connect to SSE server: %v", err)
	}
	defer res.Body.Close()

	reader := bufio.NewReader(res.Body)
	var wg sync.WaitGroup

	wg.Go(func() {
		event, data := readEvent(t, reader)
		testutil.AssertEqual(t, event, "greeting")
		testutil.AssertEqual(t, data, "Hello, world!")
	})

	// Give the client goroutine time to start listening.
	time.Sleep(50 * time.Millisecond)
	streamer.SendEvent("greeting", "Hello, world!")

	wg.Wait()
}

func TestStreamer_Broadcast(t *testing.T) {
	t.Parallel()

	streamer := NewStreamer()
	server := httptest.NewServer(streamer)
	defer server.Close()

	numClients := 3
	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := range numClients {
		go func(id int) {
			defer wg.Done()
			res, err := http.Get(server.URL)
			if err != nil {
				t.Errorf("Client %d failed to connect: %v", id, err)
				return
			}
			defer res.Body.Close()

			reader := bufio.NewReader(res.Body)
			event, data := readEvent(t, reader)
			testutil.AssertEqual(t, event, "message")
			testutil.AssertEqual(t, data, "broadcast message")
		}(i)
	}

	// Wait for clients to connect.
	for i := 0; i < 20 && streamer.ClientCount() < numClients; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	testutil.AssertEqual(t, streamer.ClientCount(), numClients)

	streamer.Send("broadcast message")
	wg.Wait()
}

func TestStreamer_ClientDisconnect(t *testing.T) {
	t.Parallel()

	streamer := NewStreamer()
	server := httptest.NewServer(streamer)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)

	var wg sync.WaitGroup
	wg.Go(func() {
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Expected context canceled error, got %v", err)
			}
			return
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	})

	// Wait for the client to connect.
	for i := 0; i < 20 && streamer.ClientCount() < 1; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	testutil.AssertEqual(t, streamer.ClientCount(), 1)

	cancel()
	wg.Wait()

	// Wait for the server to process the disconnection.
	time.Sleep(50 * time.Millisecond)
	testutil.AssertEqual(t, streamer.ClientCount(), 0)
}

func TestStreamer_SendJSON(t *testing.T) {
	t.Parallel()

	streamer := NewStreamer()
	server := httptest.NewServer(streamer)
	defer server.Close()

	type payload struct {
		ID      int    `json:"id"`
		Message string `json:"message"`
	}

	data := payload{ID: 42, Message: "test"}
	expectedJSON, _ := json.Marshal(data)

	res, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)

	var wg sync.WaitGroup
	wg.Go(func() {
		event, data := readEvent(t, reader)
		testutil.AssertEqual(t, event, "status")
		testutil.AssertEqual(t, data, string(expectedJSON))
	})

	time.Sleep(50 * time.Millisecond)
	if err := streamer.SendJSON("status", data); err != nil {
		t.Fatalf("SendJSON failed: %v", err)
	}

	wg.Wait()
}
