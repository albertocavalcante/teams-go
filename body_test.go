package teams_test

import (
	"strings"
	"testing"

	teams "github.com/albertocavalcante/teams-go"
)

func TestItemBodyTextIsNilSafe(t *testing.T) {
	t.Parallel()
	var body *teams.ItemBody
	if got := body.Text(); got != "" {
		t.Errorf("Text() on nil ItemBody = %q, want empty", got)
	}
}

func TestIdentitySetNameIsNilSafe(t *testing.T) {
	t.Parallel()
	var set *teams.IdentitySet
	if got := set.Name(); got != "" {
		t.Errorf("Name() on nil IdentitySet = %q, want empty", got)
	}
	if _, _, ok := set.Actor(); ok {
		t.Error("Actor() on nil IdentitySet reported ok = true")
	}
}

func TestPlainTextResolvesMentions(t *testing.T) {
	t.Parallel()
	msg := teams.ChatMessage{
		Body: teams.HTMLBody(`Hey <at id="0">Alice</at>, are you around?`),
		Mentions: []teams.Mention{
			{ID: 0, MentionText: "Alice Smith"},
		},
	}
	got := msg.PlainText()
	if !strings.Contains(got, "Alice Smith") {
		t.Errorf("PlainText() = %q, want it to contain the resolved mention", got)
	}
	if strings.Contains(got, "<at") {
		t.Errorf("PlainText() = %q, want no leftover markup", got)
	}
}

// A mention naming an ID absent from the message's Mentions list falls back
// to the text enclosed in the <at> tag rather than vanishing.
func TestPlainTextMentionFallsBackWhenUnresolved(t *testing.T) {
	t.Parallel()
	msg := teams.ChatMessage{
		Body: teams.HTMLBody(`Hey <at id="99">Someone</at>`),
	}
	got := msg.PlainText()
	if !strings.Contains(got, "Someone") {
		t.Errorf("PlainText() = %q, want the fallback text preserved", got)
	}
}

// The Skype mention span is a transparent wrapper around the <at> tag it
// contains; a naive renderer that separately extracts the span's own text and
// then renders the inner <at> too would double the name.
func TestPlainTextSkypeMentionSpanDoesNotDoubleName(t *testing.T) {
	t.Parallel()
	msg := teams.ChatMessage{
		Body: teams.HTMLBody(`<span itemtype="http://schema.skype.com/Mention" itemid="0"><at id="0">Alice</at></span> said hi`),
		Mentions: []teams.Mention{
			{ID: 0, MentionText: "Alice"},
		},
	}
	got := msg.PlainText()
	if strings.Count(got, "Alice") != 1 {
		t.Errorf("PlainText() = %q, want exactly one occurrence of Alice", got)
	}
}

func TestPlainTextConvertsStructuralTagsToNewlines(t *testing.T) {
	t.Parallel()
	msg := teams.ChatMessage{
		Body: teams.HTMLBody(`<div>line one</div><div>line two</div>line three<br>line four`),
	}
	got := msg.PlainText()
	lines := strings.Split(strings.TrimSpace(got), "\n")
	var nonEmpty []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty = append(nonEmpty, strings.TrimSpace(line))
		}
	}
	want := []string{"line one", "line two", "line three", "line four"}
	if len(nonEmpty) != len(want) {
		t.Fatalf("PlainText() = %q, got lines %v, want %v", got, nonEmpty, want)
	}
	for i := range want {
		if nonEmpty[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, nonEmpty[i], want[i])
		}
	}
}

func TestPlainTextStripsUnknownTags(t *testing.T) {
	t.Parallel()
	msg := teams.ChatMessage{
		Body: teams.HTMLBody(`<strong>bold</strong> and <em>italic</em>`),
	}
	got := msg.PlainText()
	if got != "bold and italic" {
		t.Errorf("PlainText() = %q, want %q", got, "bold and italic")
	}
}

func TestPlainTextUnescapesEntities(t *testing.T) {
	t.Parallel()
	msg := teams.ChatMessage{
		Body: teams.HTMLBody(`Tom &amp; Jerry &lt;3`),
	}
	got := msg.PlainText()
	if got != "Tom & Jerry <3" {
		t.Errorf("PlainText() = %q, want %q", got, "Tom & Jerry <3")
	}
}

func TestPlainTextNilBody(t *testing.T) {
	t.Parallel()
	var msg teams.ChatMessage
	if got := msg.PlainText(); got != "" {
		t.Errorf("PlainText() on a message with no body = %q, want empty", got)
	}
}

func TestInlineImagesExtraction(t *testing.T) {
	t.Parallel()
	msg := teams.ChatMessage{
		ChatID: "19:abc@thread.v2",
		Body:   teams.HTMLBody(`Look: <img src="https://graph.microsoft.com/v1.0/chats/19:abc@thread.v2/messages/123/hostedContents/hc1/$value" alt="a screenshot">`),
	}
	refs := msg.InlineImages()
	if len(refs) != 1 {
		t.Fatalf("InlineImages() = %+v, want exactly one", refs)
	}
	if refs[0].ID != "hc1" {
		t.Errorf("ID = %q, want %q", refs[0].ID, "hc1")
	}
	if refs[0].Alt != "a screenshot" {
		t.Errorf("Alt = %q, want %q", refs[0].Alt, "a screenshot")
	}
}

func TestInlineImagesNilBody(t *testing.T) {
	t.Parallel()
	var msg teams.ChatMessage
	if got := msg.InlineImages(); got != nil {
		t.Errorf("InlineImages() on a message with no body = %+v, want nil", got)
	}
}

func TestPlainTextEnforcesMaxBodyBytes(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("a", teams.MaxBodyBytes*2)
	msg := teams.ChatMessage{Body: teams.HTMLBody(huge)}
	got := msg.PlainText()
	if len(got) > teams.MaxBodyBytes {
		t.Fatalf("PlainText() length = %d, want <= %d", len(got), teams.MaxBodyBytes)
	}
}
