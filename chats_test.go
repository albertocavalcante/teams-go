package teams_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/albertocavalcante/msgraph-go/graphmock"
	teams "github.com/albertocavalcante/teams-go"
)

// TestChatsOrderedByRecentMessageAskForThePreview pins that ByRecentMessage
// adds the lastMessagePreview expansion on its own: the service refuses to
// order by a property inside an expansion it was not asked to perform, so
// asking to sort by recent message has to also ask for the preview or the
// request fails in a way this type exists to make unreachable.
func TestChatsOrderedByRecentMessageAskForThePreview(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	var expand, orderBy string
	server.Handle("GET /chats", func(r *graphmock.Request) graphmock.Response {
		expand = r.Query.Raw.Get("$expand")
		orderBy = strings.Join(r.Query.OrderBy, ",")
		return graphmock.Collection([]teams.Chat{}, "")
	})
	client := newTeamsClient(t, server)

	for _, err := range client.Chats(context.Background(), teams.ChatQuery{ByRecentMessage: true}) {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(expand, "lastMessagePreview") {
		t.Fatalf("$expand = %q, want it to include lastMessagePreview", expand)
	}
	if !strings.Contains(orderBy, teams.FieldLastMessagePreviewCreated) {
		t.Fatalf("$orderby = %q, want %q", orderBy, teams.FieldLastMessagePreviewCreated)
	}
}

// TestUnreadIsUnknownWithoutThePreview pins that Unread refuses to guess:
// with no preview, no viewpoint, or no read mark, it reports known=false
// rather than false=read, which would mark every under-fetched chat as read.
func TestUnreadIsUnknownWithoutThePreview(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		chat teams.Chat
	}{
		{"nothing populated", teams.Chat{}},
		{"preview but no viewpoint", teams.Chat{
			LastMessagePreview: &teams.MessagePreview{CreatedDateTime: time.Now()},
		}},
		{"viewpoint but no read mark and no preview", teams.Chat{
			Viewpoint: &teams.ChatViewpoint{},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, known := tc.chat.Unread(); known {
				t.Fatalf("Unread() reported known=true for %q, want false", tc.name)
			}
		})
	}
}

// TestUnreadComparesAgainstTheReadMark pins the actual comparison once both
// halves are present.
func TestUnreadComparesAgainstTheReadMark(t *testing.T) {
	t.Parallel()
	readAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	newer := teams.Chat{
		LastMessagePreview: &teams.MessagePreview{CreatedDateTime: readAt.Add(time.Hour)},
		Viewpoint:          &teams.ChatViewpoint{LastMessageReadDateTime: &readAt},
	}
	if unread, known := newer.Unread(); !known || !unread {
		t.Fatalf("Unread() = (%v, %v), want (true, true) for a message newer than the read mark", unread, known)
	}

	older := teams.Chat{
		LastMessagePreview: &teams.MessagePreview{CreatedDateTime: readAt.Add(-time.Hour)},
		Viewpoint:          &teams.ChatViewpoint{LastMessageReadDateTime: &readAt},
	}
	if unread, known := older.Unread(); !known || unread {
		t.Fatalf("Unread() = (%v, %v), want (false, true) for a message older than the read mark", unread, known)
	}
}

// TestOneOnOneChatNamesTheOtherParticipant round-trips a chat through the
// fake, which nulls topic for a oneOnOne chat the way the service does, and
// pins that DisplayName falls back to the other member -- excluding self --
// rather than surfacing the null.
func TestOneOnOneChatNamesTheOtherParticipant(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	server.Handle("GET /chats/{chat}", func(*graphmock.Request) graphmock.Response {
		return graphmock.Item(map[string]any{
			"id":       "19:a@unq.gbl.spaces",
			"chatType": "oneOnOne",
			// A topic the service would never actually send for a oneOnOne
			// chat; the shaper has to null it, or this test proves nothing.
			"topic": "a topic the service would not send",
			"members": []map[string]any{
				{"userId": "self", "displayName": "Me"},
				{"userId": "other", "displayName": "Alberto"},
			},
		})
	})
	client := newTeamsClient(t, server)

	chat, _, err := client.GetChat(context.Background(), "19:a@unq.gbl.spaces", teams.ChatQuery{WithMembers: true})
	if err != nil {
		t.Fatal(err)
	}
	if chat.Topic != "" {
		t.Fatalf("Topic = %q, want empty for a oneOnOne chat", chat.Topic)
	}
	if got := chat.DisplayName("self"); got != "Alberto" {
		t.Fatalf("DisplayName(%q) = %q, want %q", "self", got, "Alberto")
	}
}
