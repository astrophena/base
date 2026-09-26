// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package tgmarkup

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"go.astrophena.name/base/request"
	"go.astrophena.name/base/rr"
	"go.astrophena.name/base/testutil"
)

const (
	fakeToken  = "EXPUNGED"
	fakeChatID = "-3735928559" // -0xdeadbeef
)

func TestEscape(t *testing.T) {
	t.Parallel()
	const plain = `a *literal* [link](https://example.com) #tag`
	message := FromMarkdown("**" + Escape(plain) + "**")
	testutil.AssertEqual(t, message, Message{
		Text:     plain + "\n\n",
		Entities: []Entity{{Type: Bold, Length: len(plain)}},
	})
}

func TestFromMarkdownLineBreaks(t *testing.T) {
	t.Parallel()

	got := FromMarkdown("soft\nbreak  \nhard")
	testutil.AssertEqual(t, got, Message{Text: "soft\nbreak\nhard\n\n"})
}

// Updating this test:
//
//	$  TELEGRAM_TOKEN=... TELEGRAM_CHAT_ID=... go test -httprecord testdata/*.httprr
//
// (notice an extra space before command to prevent recording it in shell
// history)

func TestFromMarkdown(t *testing.T) {
	testutil.Run(t, "testdata/*.md", func(t *testing.T, match string) {
		recFile := strings.TrimSuffix(match, ".md") + ".httprr"

		rec, err := rr.Open(recFile, http.DefaultTransport)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := rec.Close(); err != nil {
				t.Error(err)
			}
		})

		token, chatID := fakeToken, fakeChatID
		if rec.Recording() {
			token = os.Getenv("TELEGRAM_TOKEN")
			chatID = os.Getenv("TELEGRAM_CHAT_ID")
		}

		rec.ScrubReq(scrubRequest)
		rec.ScrubResp(scrubResponse)

		source, err := os.ReadFile(match)
		if err != nil {
			t.Fatal(err)
		}

		body := sendMessageRequest{
			ChatID:  chatID,
			Message: FromMarkdown(string(source)),
		}

		_, err = request.Make[request.IgnoreResponse](t.Context(), request.Params{
			Method:     http.MethodPost,
			URL:        "https://api.telegram.org/bot" + token + "/sendMessage",
			Body:       body,
			HTTPClient: rec.Client(),
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

type sendMessageRequest struct {
	ChatID string `json:"chat_id"`
	Message
}

func scrubRequest(r *http.Request) error {
	r.URL.Path = "/bot" + fakeToken + "/sendMessage"
	r.URL.RawPath = ""

	body := r.Body.(*rr.Body)
	var msg sendMessageRequest
	if err := json.Unmarshal(body.Data, &msg); err != nil {
		return err
	}
	msg.ChatID = fakeChatID

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	body.Data = data
	return nil
}

func scrubResponse(b *bytes.Buffer) error {
	status, _, ok := strings.Cut(b.String(), "\r\n")
	if !ok {
		return errors.New("missing HTTP status line")
	}

	b.Reset()
	b.WriteString(status)
	b.WriteString("\r\nContent-Length: 0\r\n\r\n")
	return nil
}

func TestScrubRequest(t *testing.T) {
	const token = "private-token"
	const chatID = "-123456789"

	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Body = &rr.Body{Data: []byte(`{"chat_id":"` + chatID + `","text":"hello"}`)}

	if err := scrubRequest(req); err != nil {
		t.Fatal(err)
	}
	if got, want := req.URL.Path, "/bot"+fakeToken+"/sendMessage"; got != want {
		t.Fatalf("request path = %q; want %q", got, want)
	}
	data := req.Body.(*rr.Body).Data
	want := `{"chat_id":"` + fakeChatID + `","text":"hello"}`
	if got := string(data); got != want {
		t.Fatalf("request body = %q; want %q", got, want)
	}
	if strings.Contains(req.URL.String(), token) || bytes.Contains(data, []byte(chatID)) {
		t.Fatal("request contains private identifiers")
	}
}

func TestScrubResponse(t *testing.T) {
	b := bytes.NewBufferString("HTTP/2.0 200 OK\r\nDate: yesterday\r\nContent-Length: 6\r\n\r\nsecret")

	if err := scrubResponse(b); err != nil {
		t.Fatal(err)
	}

	if got, want := b.String(), "HTTP/2.0 200 OK\r\nContent-Length: 0\r\n\r\n"; got != want {
		t.Fatalf("response = %q; want %q", got, want)
	}
}
