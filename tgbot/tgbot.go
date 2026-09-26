// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package tgbot sends Markdown messages through the Telegram Bot API.
package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.astrophena.name/base/request"
	"go.astrophena.name/base/tgmarkup"
)

const (
	apiURL          = "https://api.telegram.org"
	messageLimit    = 4096
	captionLimit    = 1024
	mediaGroupLimit = 10
)

// Config identifies a bot and its request settings. Attempts defaults to one.
type Config struct {
	Token      string
	HTTPClient *http.Client
	Attempts   int
	UserAgent  string
	Logger     *slog.Logger
}

// Client sends messages using one bot token.
type Client struct {
	token string
	httpc *http.Client
	tries int
	scrub *strings.Replacer
	agent string
	log   *slog.Logger
	sleep func(context.Context, time.Duration) bool
}

// New returns a client for cfg.Token.
func New(cfg Config) *Client {
	httpc := cfg.HTTPClient
	if httpc == nil {
		httpc = request.DefaultClient
	}

	c := &Client{
		token: cfg.Token,
		httpc: httpc,
		tries: max(cfg.Attempts, 1),
		agent: cfg.UserAgent,
		log:   cfg.Logger,
		sleep: sleep,
	}
	if cfg.Token != "" {
		c.scrub = strings.NewReplacer(cfg.Token, "[EXPUNGED]")
	}
	return c
}

// Button links to a URL from an inline keyboard.
type Button struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Media is a photo or video URL. Type containing "video", or a URL ending in
// ".mp4", selects video.
type Media struct {
	Type string
	URL  string
}

// Message is an outgoing Markdown message. ChatID is a numeric ID or channel
// name. Keyboard goes on the final text message or single media item; media
// groups cannot have keyboards.
type Message struct {
	ChatID             string
	ThreadID           int64
	Text               string
	Media              []Media
	Keyboard           [][]Button
	DisableLinkPreview bool
}

// Send sends msg and returns the IDs sent, including IDs sent before an error.
// A message with no text or media sends nothing.
func (c *Client) Send(ctx context.Context, msg Message) ([]int, error) {
	if c.token == "" {
		return nil, errors.New("missing Telegram bot token")
	}
	if msg.ChatID == "" {
		return nil, errors.New("missing Telegram chat ID")
	}

	limit := messageLimit
	if len(msg.Media) > 0 {
		limit = captionLimit
	}
	parts := split(tgmarkup.FromMarkdown(msg.Text), limit)

	var ids []int
	if len(msg.Media) > 0 {
		var caption tgmarkup.Message
		if len(parts) > 0 {
			caption, parts = parts[0], parts[1:]
		}

		for start := 0; start < len(msg.Media); start += mediaGroupLimit {
			end := min(start+mediaGroupLimit, len(msg.Media))
			batch := msg.Media[start:end]

			if len(batch) == 1 {
				var keyboard [][]Button
				if end == len(msg.Media) && len(parts) == 0 {
					keyboard = msg.Keyboard
				}

				id, err := c.sendMedia(ctx, msg, batch[0], caption, keyboard)
				if err != nil {
					return ids, err
				}
				ids = append(ids, id)
			} else {
				sent, err := c.sendMediaGroup(ctx, msg, batch, caption)
				if err != nil {
					return ids, err
				}
				ids = append(ids, sent...)
			}
			caption = tgmarkup.Message{}
		}
	}

	for i, part := range parts {
		req := textRequest{
			ChatID:   msg.ChatID,
			ThreadID: msg.ThreadID,
			Message:  part,
			LinkPreview: linkPreview{
				Disabled: msg.DisableLinkPreview,
			},
		}
		if i == len(parts)-1 && len(msg.Keyboard) > 0 {
			req.Keyboard = &keyboardMarkup{
				InlineKeyboard: msg.Keyboard,
			}
		}

		sent, err := call[sentMessage](ctx, c, "sendMessage", req)
		if err != nil {
			return ids, err
		}
		ids = append(ids, sent.MessageID)
	}

	return ids, nil
}

type textRequest struct {
	ChatID      string          `json:"chat_id"`
	ThreadID    int64           `json:"message_thread_id,omitempty"`
	LinkPreview linkPreview     `json:"link_preview_options"`
	Keyboard    *keyboardMarkup `json:"reply_markup,omitempty"`
	tgmarkup.Message
}

type linkPreview struct {
	Disabled bool `json:"is_disabled"`
}

type keyboardMarkup struct {
	InlineKeyboard [][]Button `json:"inline_keyboard"`
}

type captionPayload struct {
	Caption         string            `json:"caption,omitempty"`
	CaptionEntities []tgmarkup.Entity `json:"caption_entities,omitempty"`
}

func caption(msg tgmarkup.Message) captionPayload {
	return captionPayload{
		Caption:         msg.Text,
		CaptionEntities: msg.Entities,
	}
}

type mediaRequest struct {
	ChatID   string          `json:"chat_id"`
	ThreadID int64           `json:"message_thread_id,omitempty"`
	Photo    string          `json:"photo,omitempty"`
	Video    string          `json:"video,omitempty"`
	Keyboard *keyboardMarkup `json:"reply_markup,omitempty"`
	captionPayload
}

type inputMedia struct {
	Type  string `json:"type"`
	Media string `json:"media"`
	captionPayload
}

type mediaGroupRequest struct {
	ChatID   string       `json:"chat_id"`
	ThreadID int64        `json:"message_thread_id,omitempty"`
	Media    []inputMedia `json:"media"`
}

func mediaType(m Media) string {
	if strings.Contains(m.Type, "video") || strings.HasSuffix(m.URL, ".mp4") {
		return "video"
	}
	return "photo"
}

func (c *Client) sendMedia(ctx context.Context, msg Message, media Media, text tgmarkup.Message, buttons [][]Button) (int, error) {
	req := mediaRequest{
		ChatID:         msg.ChatID,
		ThreadID:       msg.ThreadID,
		captionPayload: caption(text),
	}
	if len(buttons) > 0 {
		req.Keyboard = &keyboardMarkup{
			InlineKeyboard: buttons,
		}
	}

	method := "sendPhoto"
	if mediaType(media) == "video" {
		method = "sendVideo"
		req.Video = media.URL
	} else {
		req.Photo = media.URL
	}

	sent, err := call[sentMessage](ctx, c, method, req)
	return sent.MessageID, err
}

func (c *Client) sendMediaGroup(ctx context.Context, msg Message, media []Media, text tgmarkup.Message) ([]int, error) {
	req := mediaGroupRequest{
		ChatID:   msg.ChatID,
		ThreadID: msg.ThreadID,
		Media:    make([]inputMedia, 0, len(media)),
	}
	for i, m := range media {
		item := inputMedia{
			Type:  mediaType(m),
			Media: m.URL,
		}
		if i == 0 {
			item.captionPayload = caption(text)
		}
		req.Media = append(req.Media, item)
	}

	sent, err := call[[]sentMessage](ctx, c, "sendMediaGroup", req)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(sent))
	for _, item := range sent {
		ids = append(ids, item.MessageID)
	}
	return ids, nil
}

type sentMessage struct {
	MessageID int `json:"message_id"`
}

type response[T any] struct {
	OK          bool       `json:"ok"`
	Result      T          `json:"result"`
	ErrorCode   int        `json:"error_code"`
	Description string     `json:"description"`
	Parameters  parameters `json:"parameters"`
}

type parameters struct {
	RetryAfter int `json:"retry_after"`
}

type apiError struct {
	code        int
	description string
	retryAfter  time.Duration
}

func (e *apiError) Error() string {
	return fmt.Sprintf("telegram Bot API returned %d: %s", e.code, e.description)
}

// RetryAfter returns Telegram's requested delay, or zero if err does not
// contain a Telegram rate-limit response.
func RetryAfter(err error) time.Duration {
	if e, ok := errors.AsType[*apiError](err); ok {
		return e.retryAfter
	}
	return 0
}

func call[T any](ctx context.Context, c *Client, method string, body any) (T, error) {
	var zero T
	for attempt := 1; ; attempt++ {
		result, err := callOnce[T](ctx, c, method, body)
		if err == nil {
			return result, nil
		}

		delay := RetryAfter(err)
		if attempt >= c.tries || delay <= 0 {
			return zero, err
		}
		if c.log != nil {
			c.log.Warn("sending rate limited, waiting", slog.String("method", method), slog.Duration("wait", delay))
		}
		if !c.sleep(ctx, delay) {
			return zero, ctx.Err()
		}
	}
}

func callOnce[T any](ctx context.Context, c *Client, method string, body any) (T, error) {
	var zero T
	var headers map[string]string
	if c.agent != "" {
		headers = map[string]string{"User-Agent": c.agent}
	}

	resp, err := request.Make[response[T]](ctx, request.Params{
		Method:     http.MethodPost,
		URL:        apiURL + "/bot" + c.token + "/" + method,
		Body:       body,
		Headers:    headers,
		HTTPClient: c.httpc,
		Scrubber:   c.scrub,
	})
	if err != nil {
		var status *request.StatusError
		if errors.As(err, &status) && json.Unmarshal(status.Body, &resp) == nil && !resp.OK {
			return zero, responseError(c, resp)
		}
		return zero, fmt.Errorf("call %s: %w", method, err)
	}
	if !resp.OK {
		return zero, responseError(c, resp)
	}
	return resp.Result, nil
}

func responseError[T any](c *Client, resp response[T]) error {
	desc := resp.Description
	if c.scrub != nil {
		desc = c.scrub.Replace(desc)
	}
	return &apiError{
		code:        resp.ErrorCode,
		description: desc,
		retryAfter:  time.Duration(resp.Parameters.RetryAfter) * time.Second,
	}
}

func sleep(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
