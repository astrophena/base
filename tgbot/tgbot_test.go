// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package tgbot

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.astrophena.name/base/testutil"
	"go.astrophena.name/base/tgmarkup"
)

func TestSendText(t *testing.T) {
	t.Parallel()

	var got textRequest
	httpc := testutil.MockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botsecret/sendMessage" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got != "test-sender" {
			t.Errorf("User-Agent = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	c := New(Config{
		Token:      "secret",
		HTTPClient: httpc,
		UserAgent:  "test-sender",
	})

	ids, err := c.Send(t.Context(), Message{
		ChatID:   "-10020",
		ThreadID: 7,
		Text:     "**Hello**",
		Keyboard: [][]Button{{{
			Text: "Open",
			URL:  "https://example.com",
		}}},
		DisableLinkPreview: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertEqual(t, ids, []int{42})
	testutil.AssertEqual(t, got, textRequest{
		ChatID:   "-10020",
		ThreadID: 7,
		Message: tgmarkup.Message{
			Text: "Hello\n\n",
			Entities: []tgmarkup.Entity{{
				Type:   tgmarkup.Bold,
				Length: 5,
			}},
		},
		LinkPreview: linkPreview{
			Disabled: true,
		},
		Keyboard: &keyboardMarkup{
			InlineKeyboard: [][]Button{{{
				Text: "Open",
				URL:  "https://example.com",
			}}},
		},
	})
}

func TestSendSplitsRenderedText(t *testing.T) {
	t.Parallel()

	var requests []textRequest
	httpc := testutil.MockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req textRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, req)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	c := New(Config{
		Token:      "secret",
		HTTPClient: httpc,
	})

	_, err := c.Send(t.Context(), Message{
		ChatID: "20",
		Text:   "**" + strings.Repeat("😀", 2049) + "**",
		Keyboard: [][]Button{{{
			Text: "Open",
			URL:  "https://example.com",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d; want 2", len(requests))
	}
	if got := len([]rune(requests[0].Text)); got != 2048 {
		t.Errorf("first chunk runes = %d; want 2048", got)
	}
	if requests[0].Keyboard != nil || requests[1].Keyboard == nil {
		t.Error("keyboard is not on final chunk")
	}
	testutil.AssertEqual(t, requests[0].Entities, []tgmarkup.Entity{{
		Type:   tgmarkup.Bold,
		Length: 4096,
	}})
	testutil.AssertEqual(t, requests[1].Entities, []tgmarkup.Entity{{
		Type:   tgmarkup.Bold,
		Length: 2,
	}})
}

func TestSendMediaGroupWithSingletonTail(t *testing.T) {
	t.Parallel()

	var methods []string
	httpc := testutil.MockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.URL.Path)
		switch len(methods) {
		case 1:
			var req mediaGroupRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if len(req.Media) != 10 || req.Media[0].Caption != "Caption\n\n" {
				t.Errorf("first media batch = %+v", req)
			}
			items := make([]sentMessage, 10)
			for i := range items {
				items[i].MessageID = i + 1
			}
			_ = json.NewEncoder(w).Encode(struct {
				OK     bool          `json:"ok"`
				Result []sentMessage `json:"result"`
			}{
				OK:     true,
				Result: items,
			})
		case 2:
			var req mediaRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Photo == "" || req.Keyboard == nil || req.Caption != "" {
				t.Errorf("last media item = %+v", req)
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":11}}`))
		default:
			t.Errorf("unexpected request %d", len(methods))
		}
	}))
	c := New(Config{
		Token:      "secret",
		HTTPClient: httpc,
	})
	media := make([]Media, 11)
	for i := range media {
		media[i] = Media{
			URL: "https://example.com/photo.jpg",
		}
	}

	ids, err := c.Send(t.Context(), Message{
		ChatID: "20",
		Text:   "Caption",
		Media:  media,
		Keyboard: [][]Button{{{
			Text: "Open",
			URL:  "https://example.com",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertEqual(t, methods, []string{"/botsecret/sendMediaGroup", "/botsecret/sendPhoto"})
	testutil.AssertEqual(t, ids, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})
}

func TestSendVideo(t *testing.T) {
	t.Parallel()

	var got mediaRequest
	httpc := testutil.MockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botsecret/sendVideo" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	}))
	c := New(Config{
		Token:      "secret",
		HTTPClient: httpc,
	})

	ids, err := c.Send(t.Context(), Message{
		ChatID: "20",
		Text:   "Caption",
		Media: []Media{{
			Type: "video/mp4",
			URL:  "https://example.com/video",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertEqual(t, ids, []int{7})
	if got.Video != "https://example.com/video" || got.Caption != "Caption\n\n" {
		t.Errorf("video request = %+v", got)
	}
}

func TestSendReturnsPartialIDs(t *testing.T) {
	t.Parallel()

	var calls int
	httpc := testutil.MockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"bad request"}`))
	}))
	c := New(Config{
		Token:      "secret",
		HTTPClient: httpc,
	})

	ids, err := c.Send(t.Context(), Message{
		ChatID: "20",
		Text:   strings.Repeat("a", 4096),
	})
	if err == nil {
		t.Fatal("Send returned nil error")
	}
	testutil.AssertEqual(t, ids, []int{7})
}

func TestSplitMessage(t *testing.T) {
	t.Parallel()

	msg := tgmarkup.Message{
		Text: "first\nsecond",
		Entities: []tgmarkup.Entity{{
			Type:   tgmarkup.Bold,
			Length: 12,
		}},
	}
	testutil.AssertEqual(t, split(msg, 8), []tgmarkup.Message{
		{
			Text: "first\n",
			Entities: []tgmarkup.Entity{{
				Type:   tgmarkup.Bold,
				Length: 6,
			}},
		},
		{
			Text: "second",
			Entities: []tgmarkup.Entity{{
				Type:   tgmarkup.Bold,
				Length: 6,
			}},
		},
	})

	msg = tgmarkup.Message{
		Text: strings.Repeat("😀", 513),
		Entities: []tgmarkup.Entity{{
			Type:   tgmarkup.Bold,
			Length: 1026,
		}},
	}
	parts := split(msg, 1024)
	if len(parts) != 2 || len([]rune(parts[0].Text)) != 512 || len([]rune(parts[1].Text)) != 1 {
		t.Fatalf("caption split = %+v", parts)
	}
	testutil.AssertEqual(t, parts[0].Entities, []tgmarkup.Entity{{
		Type:   tgmarkup.Bold,
		Length: 1024,
	}})
	testutil.AssertEqual(t, parts[1].Entities, []tgmarkup.Entity{{
		Type:   tgmarkup.Bold,
		Length: 2,
	}})
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()

	const token = "private-token"
	var calls int
	httpc := testutil.MockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"` + token + `","parameters":{"retry_after":17}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	c := New(Config{
		Token:      token,
		HTTPClient: httpc,
		Attempts:   2,
	})
	var waited time.Duration
	c.sleep = func(_ context.Context, d time.Duration) bool {
		waited = d
		return true
	}

	ids, err := c.Send(t.Context(), Message{
		ChatID: "20",
		Text:   "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertEqual(t, ids, []int{42})
	if calls != 2 || waited != 17*time.Second {
		t.Fatalf("calls = %d, waited = %s", calls, waited)
	}

	c.tries = 1
	calls = 0
	_, err = c.Send(t.Context(), Message{
		ChatID: "20",
		Text:   "hello",
	})
	if got := RetryAfter(err); got != 17*time.Second {
		t.Fatalf("RetryAfter = %s; want 17s", got)
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("error contains bot token: %v", err)
	}
}
