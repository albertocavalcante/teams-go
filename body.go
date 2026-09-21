package teams

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Bounds on how much of a message body this package will render as text.
const (
	// MaxBodyDepth bounds how many levels of tag nesting [ChatMessage.PlainText]
	// tracks before it stops counting further opens. Nesting past the bound
	// is still stripped like any other tag; only the counter itself saturates,
	// so a body built from a huge run of open tags cannot grow unbounded
	// internal state.
	MaxBodyDepth = 64
	// MaxBodyBytes bounds the rendered output of [ItemBody.Text] and
	// [ChatMessage.PlainText]. Content past the bound is truncated, never
	// rejected: a caller rendering a preview would rather see the first
	// megabyte of an oversized message than an error.
	MaxBodyBytes = 1 << 20
)

// HostedContentRef is an inline image [ChatMessage.InlineImages] found in a
// message body, referencing the chat or channel's hostedContents collection
// rather than carrying the image bytes itself.
type HostedContentRef struct {
	ID  HostedContentID
	URL string
	Alt string
}

// Text returns the body's content, unescaped of HTML entities when the
// content type is HTML but with markup left untouched otherwise -- this is
// the raw body, not a rendering of it. It is nil-safe and does no mention
// resolution, since an [ItemBody] on its own carries no mentions list to
// resolve against; see [ChatMessage.PlainText] for that.
func (b *ItemBody) Text() string {
	if b == nil {
		return ""
	}
	return b.Content
}

// PlainText renders the message body as text, resolving <at id="N"> mentions
// against m.Mentions, converting <br>, <p>, and <div> to newlines, replacing
// an inline image with a bracketed marker, and stripping every other tag. It
// does no I/O and is a pure function of m: everything it needs is already in
// the value.
func (m ChatMessage) PlainText() string {
	if m.Body == nil {
		return ""
	}
	return renderBody(m.Body.Content, m.Body.ContentType, m.Mentions)
}

// InlineImages returns the hosted-content images referenced by <img> tags in
// the message body, in document order.
func (m ChatMessage) InlineImages() []HostedContentRef {
	if m.Body == nil {
		return nil
	}
	return extractInlineImages(truncateBytes(m.Body.Content, MaxBodyBytes))
}

// renderBody is the shared scanner behind [ChatMessage.PlainText]. A nil or
// empty mentions list is fine: an <at> tag with nothing to resolve against
// falls back to its enclosed text, which is what [ItemBody.Text] relies on
// implicitly by never calling this with mentions at all.
func renderBody(content string, contentType BodyType, mentions []Mention) string {
	content = truncateBytes(content, MaxBodyBytes)
	if contentType != BodyTypeHTML {
		return html.UnescapeString(content)
	}
	return scanHTML(content, mentions)
}

// scanHTML walks content once, left to right, with no recursion: depth is a
// saturating counter, not a call stack, so a body built from a huge run of
// unclosed tags is bounded work and bounded memory rather than a stack
// overflow that recover() cannot catch.
func scanHTML(content string, mentions []Mention) string {
	var out strings.Builder
	var atCapture *strings.Builder
	var atID string
	depth := 0

	i := 0
	n := len(content)
	for i < n {
		if out.Len() >= MaxBodyBytes && atCapture == nil {
			break
		}
		if content[i] != '<' {
			j := i
			for j < n && content[j] != '<' {
				j++
			}
			writeBodyText(&out, atCapture, html.UnescapeString(content[i:j]))
			i = j
			continue
		}

		end := strings.IndexByte(content[i:], '>')
		if end < 0 {
			// An unterminated tag: there is no closing '>' anywhere in the
			// rest of the content, so treat what remains as literal text
			// rather than scanning forever for a byte that will never come.
			writeBodyText(&out, atCapture, html.UnescapeString(content[i:]))
			break
		}
		tag := content[i : i+end+1]
		i += end + 1

		inner := strings.TrimSuffix(strings.TrimPrefix(tag, "<"), ">")
		closing := strings.HasPrefix(inner, "/")
		inner = strings.TrimPrefix(inner, "/")
		name := strings.ToLower(tagName(inner))

		switch name {
		case "at":
			if !closing {
				if atCapture == nil {
					atCapture = &strings.Builder{}
					atID = attrValue(inner, "id")
				}
				// A nested <at> -- real Teams output never produces one --
				// does not start a second capture; its own markup is dropped
				// like any other tag, and its closing tag flushes the single
				// outer capture early. That is a defined, bounded outcome
				// for input this package never sees in practice, not an
				// attempt to parse it correctly.
			} else if atCapture != nil {
				text := atCapture.String()
				if resolved, ok := resolveMention(atID, mentions); ok {
					text = resolved
				}
				atCapture, atID = nil, ""
				writeBodyText(&out, nil, text)
			}
		case "img":
			if !closing {
				if ref, ok := hostedContentFromImg(inner); ok {
					writeBodyText(&out, atCapture, "["+ref.Alt+"]")
				}
			}
		case "br":
			writeBodyText(&out, atCapture, "\n")
		case "p", "div":
			if closing {
				writeBodyText(&out, atCapture, "\n")
			}
		default:
			// span and everything else are transparent: the tag itself
			// contributes nothing, and its content flows through the loop
			// exactly as it would outside the tag. Depth only tracks how
			// deep the current, unrecognized nesting is; nothing branches on
			// it, so saturating it at MaxBodyDepth changes no behavior.
			if !closing {
				if depth < MaxBodyDepth {
					depth++
				}
			} else if depth > 0 {
				depth--
			}
		}
	}
	// An <at> left open by malformed or truncated input still has something
	// to say: emit its captured text rather than discarding it silently.
	if atCapture != nil {
		writeBodyText(&out, nil, atCapture.String())
	}
	return out.String()
}

// writeBodyText appends text to capture when non-nil, otherwise to out,
// truncating at MaxBodyBytes. Writing into a capture buffer is not truncated
// here because the buffer can never exceed the overall content length, which
// [renderBody] already bounded to MaxBodyBytes before scanning began.
func writeBodyText(out, capture *strings.Builder, text string) {
	if capture != nil {
		capture.WriteString(text)
		return
	}
	remaining := MaxBodyBytes - out.Len()
	if remaining <= 0 {
		return
	}
	if len(text) > remaining {
		text = truncateBytes(text, remaining)
	}
	out.WriteString(text)
}

// resolveMention finds the mention whose ID matches id and returns its
// display text.
func resolveMention(id string, mentions []Mention) (string, bool) {
	if id == "" {
		return "", false
	}
	for _, mention := range mentions {
		if strconv.Itoa(int(mention.ID)) != id {
			continue
		}
		if mention.MentionText != "" {
			return mention.MentionText, true
		}
		if name := mention.Mentioned.name(); name != "" {
			return name, true
		}
		return "", false
	}
	return "", false
}

// extractInlineImages scans content for <img> tags and returns the hosted
// content each one references, in document order. Content not shaped like an
// <img src="…/hostedContents/{id}/$value"> is skipped rather than reported as
// a reference to nothing.
func extractInlineImages(content string) []HostedContentRef {
	var refs []HostedContentRef
	i := 0
	n := len(content)
	for i < n {
		idx := strings.Index(content[i:], "<img")
		if idx < 0 {
			break
		}
		start := i + idx
		end := strings.IndexByte(content[start:], '>')
		if end < 0 {
			break
		}
		inner := content[start+1 : start+end]
		if ref, ok := hostedContentFromImg(inner); ok {
			refs = append(refs, ref)
		}
		i = start + end + 1
	}
	return refs
}

// hostedContentFromImg builds a [HostedContentRef] from an <img> tag's inner
// attribute text, reporting false when its src is not a hostedContents URL.
func hostedContentFromImg(inner string) (HostedContentRef, bool) {
	src := attrValue(inner, "src")
	id, ok := hostedContentIDFromURL(src)
	if !ok {
		return HostedContentRef{}, false
	}
	return HostedContentRef{ID: HostedContentID(id), URL: src, Alt: attrValue(inner, "alt")}, true
}

// hostedContentIDFromURL pulls the hosted content id out of a URL shaped like
// ".../hostedContents/{id}/$value".
func hostedContentIDFromURL(src string) (string, bool) {
	const marker = "/hostedContents/"
	idx := strings.Index(src, marker)
	if idx < 0 {
		return "", false
	}
	rest := src[idx+len(marker):]
	if end := strings.IndexByte(rest, '/'); end >= 0 {
		rest = rest[:end]
	}
	if rest == "" {
		return "", false
	}
	return rest, true
}

// tagName returns the element name at the start of a tag's inner text, such
// as "img" from `img src="…" alt="…"`.
func tagName(inner string) string {
	inner = strings.TrimSpace(inner)
	end := 0
	for end < len(inner) && !isHTMLSpace(inner[end]) && inner[end] != '/' {
		end++
	}
	return inner[:end]
}

// attrValue returns the value of a key="value" or key='value' attribute
// inside a tag's inner text, or the empty string when key is absent or
// malformed. Comparison is case-insensitive, as HTML attribute names are.
func attrValue(inner, key string) string {
	lowered := strings.ToLower(inner)
	keyLower := strings.ToLower(key)
	i := 0
	for i < len(lowered) {
		idx := strings.Index(lowered[i:], keyLower)
		if idx < 0 {
			return ""
		}
		pos := i + idx
		// i strictly increases on every path through this loop, since pos is
		// never less than i and this always advances past pos: the search is
		// bounded by len(lowered) regardless of what key or inner contain.
		i = pos + 1
		if pos > 0 && !isHTMLSpace(lowered[pos-1]) {
			continue
		}
		j := pos + len(keyLower)
		for j < len(lowered) && isHTMLSpace(lowered[j]) {
			j++
		}
		if j >= len(lowered) || lowered[j] != '=' {
			continue
		}
		j++
		for j < len(lowered) && isHTMLSpace(lowered[j]) {
			j++
		}
		if j >= len(inner) {
			return ""
		}
		quote := inner[j]
		if quote != '"' && quote != '\'' {
			return ""
		}
		j++
		start := j
		for j < len(inner) && inner[j] != quote {
			j++
		}
		if j >= len(inner) {
			return ""
		}
		return html.UnescapeString(inner[start:j])
	}
	return ""
}

func isHTMLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// truncateBytes cuts s to at most limit bytes, backing off to the last full
// UTF-8 rune boundary so truncation cannot split a multi-byte character into
// invalid UTF-8.
func truncateBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	for s != "" && !utf8.RuneStart(s[len(s)-1]) {
		s = s[:len(s)-1]
	}
	return s
}
