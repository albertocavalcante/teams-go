package teams_test

import (
	"strings"
	"testing"

	teams "github.com/albertocavalcante/teams-go"
)

// FuzzPlainTextIsBounded is the property this package cannot afford to get
// wrong: PlainText scans untrusted server-controlled HTML with no recursion,
// so the only way it can misbehave under hostile input is by growing its
// output without bound. A crash here would mean the bound in [body.go] has a
// hole; a fuzzer finds that far faster than a human reading the scanner can.
func FuzzPlainTextIsBounded(f *testing.F) {
	seeds := []string{
		"",
		"plain text, no tags at all",
		strings.Repeat("<div>", 10000),
		strings.Repeat("<div>x</div>", 5000),
		"<at id=\"99\">a mention naming an id nobody sent</at>",
		"<at id=\"0\">unterminated tag with no closing angle bracket",
		"<span itemtype=\"http://schema.skype.com/Mention\"><at id=\"0\">Alice</at></span>",
		"<img src=\"https://graph.microsoft.com/v1.0/chats/1/messages/1/hostedContents/hc1/$value\">",
		"<br><br><br><p></p><div></div>",
		strings.Repeat("&amp;", 20000),
		"<at id=\"0\"><at id=\"1\"><at id=\"2\">nested mentions</at></at></at>",
		"<",
		"<<<<<<<<<<<<<<<<",
		strings.Repeat("a", 1<<21),
	}
	for _, seed := range seeds {
		f.Add(seed, "html")
	}
	for _, seed := range seeds {
		f.Add(seed, "text")
	}

	f.Fuzz(func(t *testing.T, body string, contentType string) {
		ct := teams.BodyTypeHTML
		if contentType != "html" {
			ct = teams.BodyTypeText
		}
		msg := teams.ChatMessage{
			Body: &teams.ItemBody{ContentType: ct, Content: body},
			Mentions: []teams.Mention{
				{ID: 0, MentionText: "Known Mention"},
			},
		}

		got := msg.PlainText()
		if len(got) > teams.MaxBodyBytes {
			t.Fatalf("PlainText() length = %d, want <= %d (input len %d)", len(got), teams.MaxBodyBytes, len(body))
		}

		// InlineImages must not panic either, and must not report more
		// references than the input could plausibly contain <img> tags for.
		refs := msg.InlineImages()
		if len(refs) > strings.Count(body, "<img") {
			t.Fatalf("InlineImages() returned %d refs for %d <img> occurrences", len(refs), strings.Count(body, "<img"))
		}
	})
}
