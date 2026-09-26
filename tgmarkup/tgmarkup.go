// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package tgmarkup converts a subset of Markdown to Telegram message text and
// entities.
//
// It supports paragraphs, headings, tight lists, block quotes, fenced and
// indented code blocks, and thematic breaks. Headings become bold text, list
// items become bullets, and thematic breaks become a horizontal bar.
//
// Inline formatting includes emphasis, strong emphasis, strikethrough,
// code, links, and automatic links. Soft and hard line breaks become newlines.
//
// For example:
//
//	# Update
//
//	**Ready** to [read](https://example.com).
//
//	- First item
//	- Second item
//
//	> A short note.
package tgmarkup

import (
	"regexp"
	"strings"
	"unicode/utf16"

	"rsc.io/markdown"
)

// Message contains Telegram message text and its formatting entities.
// See https://core.telegram.org/bots/api#message for more information.
type Message struct {
	Text     string   `json:"text" starlark:"text"`
	Entities []Entity `json:"entities,omitempty" starlark:"entities"`
}

// Type identifies a Telegram message entity type.
type Type string

// Telegram message entity types.
// See https://core.telegram.org/bots/api#messageentity for a complete list of
// supported types.
const (
	Mention              Type = "mention"      // @username
	Hashtag              Type = "hashtag"      // #hashtag
	Cashtag              Type = "cashtag"      // $USD
	BotCommand           Type = "bot_command"  // /start@jobs_bot
	URL                  Type = "url"          // https://telegram.org
	Email                Type = "email"        // do-not-reply@telegram.org
	PhoneNumber          Type = "phone_number" // +1-212-555-0123
	Bold                 Type = "bold"
	Italic               Type = "italic"
	Underline            Type = "underline"
	Strikethrough        Type = "strikethrough"
	Spoiler              Type = "spoiler"
	Blockquote           Type = "blockquote"
	ExpandableBlockquote Type = "expandable_blockquote"
	Code                 Type = "code" // monospaced text
	Pre                  Type = "pre"  // monospaced block
	TextLink             Type = "text_link"
	TextMention          Type = "text_mention"
	CustomEmoji          Type = "custom_emoji"
)

// Entity defines the type and location of a formatted part of the message text.
// See https://core.telegram.org/bots/api#messageentity.
type Entity struct {
	Type Type `json:"type" starlark:"type"`
	// Offset in UTF-16 code units to the start of the entity.
	Offset int `json:"offset" starlark:"offset"`
	// Length of the entity in UTF-16 code units.
	Length int `json:"length" starlark:"length"`
	// URL is the target of a TextLink entity.
	URL string `json:"url,omitempty" starlark:"url"`
	// Language identifies the programming language of a Pre entity.
	Language string `json:"language,omitempty" starlark:"language"`
}

var markdownParser = &markdown.Parser{
	Strikethrough:      true,
	AutoLinkText:       true,
	AutoLinkAssumeHTTP: true,
	SmartDot:           true,
	SmartDash:          true,
	SmartQuote:         true,
}

// FromMarkdown converts Markdown text to a [Message]. Entity offsets and
// lengths are measured in UTF-16 code units.
func FromMarkdown(source string) Message {
	document := markdownParser.Parse(source)

	var (
		text     strings.Builder
		entities []Entity
	)

	for _, block := range document.Blocks {
		convertBlock(block, &text, &entities, false)
	}

	return Message{
		Text:     text.String(),
		Entities: entities,
	}
}

// Escape returns text that Markdown parses literally. Use it when inserting
// untrusted or otherwise plain text into a Markdown message.
func Escape(text string) string {
	const punctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
	var escaped strings.Builder
	escaped.Grow(len(text))

	for _, character := range text {
		if strings.ContainsRune(punctuation, character) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(character)
	}
	return escaped.String()
}

var spacesRE = regexp.MustCompile(`[ ]+`)

func collapseSpaces(text string) string { return spacesRE.ReplaceAllString(text, " ") }

func convertBlock(block markdown.Block, text *strings.Builder, entities *[]Entity, lastInQuote bool) {
	switch block := block.(type) {
	case *markdown.Paragraph:
		convertInlines(block.Text.Inline, text, entities)
		if lastInQuote {
			text.WriteString("\n")
			return
		}
		text.WriteString("\n\n")
	case *markdown.Text:
		// This is a Block for tight list items.
		text.WriteString("• ")
		convertInlines(block.Inline, text, entities)
		text.WriteString("\n")
	case *markdown.Quote:
		offset := utf16Len(text.String())
		for i, child := range block.Blocks {
			convertBlock(child, text, entities, i == len(block.Blocks)-1)
		}
		*entities = append(*entities, Entity{
			Type:   Blockquote,
			Offset: offset,
			Length: utf16Len(text.String()) - offset,
		})
	case *markdown.CodeBlock:
		offset := utf16Len(text.String())
		for _, line := range block.Text {
			text.WriteString(line)
			text.WriteString("\n")
		}
		text.WriteString("\n")

		entity := Entity{
			Type:   Pre,
			Offset: offset,
			Length: utf16Len(text.String()) - offset - 1,
		}
		if block.Info != "" {
			entity.Language = block.Info
		}
		*entities = append(*entities, entity)
	case *markdown.Heading:
		offset := utf16Len(text.String())
		convertInlines(block.Text.Inline, text, entities)
		text.WriteString("\n\n")
		*entities = append(*entities, Entity{
			Type:   Bold,
			Offset: offset,
			Length: utf16Len(text.String()) - offset - 1,
		})
	case *markdown.List:
		for i, itemBlock := range block.Items {
			item := itemBlock.(*markdown.Item)
			for _, child := range item.Blocks {
				convertBlock(child, text, entities, false)
			}
			if i == len(block.Items)-1 {
				text.WriteString("\n")
			}
		}
	case *markdown.ThematicBreak:
		text.WriteString("⸻\n")
	}
}

func convertInlines(inlines markdown.Inlines, text *strings.Builder, entities *[]Entity) {
	for _, inline := range inlines {
		convertInline(inline, text, entities)
	}
}

func convertInline(inline markdown.Inline, text *strings.Builder, entities *[]Entity) {
	switch inline := inline.(type) {
	case *markdown.Plain:
		// Collapse runs of spaces in plain text before writing it.
		// See https://old.reddit.com/r/GoogleGeminiAI/comments/1d1z9l3/google_gemini_15_output_contains_random_extra/.
		text.WriteString(collapseSpaces(inline.Text))
	case *markdown.Escaped:
		text.WriteString(inline.Text)
	case *markdown.Strong:
		offset := utf16Len(text.String())
		convertInlines(inline.Inner, text, entities)
		*entities = append(*entities, Entity{
			Type:   Bold,
			Offset: offset,
			Length: utf16Len(text.String()) - offset,
		})
	case *markdown.Emph:
		offset := utf16Len(text.String())
		convertInlines(inline.Inner, text, entities)
		*entities = append(*entities, Entity{
			Type:   Italic,
			Offset: offset,
			Length: utf16Len(text.String()) - offset,
		})
	case *markdown.Link:
		offset := utf16Len(text.String())
		convertInlines(inline.Inner, text, entities)
		*entities = append(*entities, Entity{
			Type:   TextLink,
			Offset: offset,
			Length: utf16Len(text.String()) - offset,
			URL:    inline.URL,
		})
	case *markdown.AutoLink:
		offset := utf16Len(text.String())
		text.WriteString(collapseSpaces(inline.Text))
		*entities = append(*entities, Entity{
			Type:   URL,
			Offset: offset,
			Length: utf16Len(text.String()) - offset,
		})
	case *markdown.Code:
		offset := utf16Len(text.String())
		text.WriteString(inline.Text)
		*entities = append(*entities, Entity{
			Type:   Code,
			Offset: offset,
			Length: utf16Len(text.String()) - offset,
		})
	case *markdown.Del:
		offset := utf16Len(text.String())
		convertInlines(inline.Inner, text, entities)
		*entities = append(*entities, Entity{
			Type:   Strikethrough,
			Offset: offset,
			Length: utf16Len(text.String()) - offset,
		})
	case *markdown.SoftBreak, *markdown.HardBreak:
		text.WriteString("\n")
	}
}

func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}
