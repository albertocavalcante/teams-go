package teams_test

import (
	"context"
	"testing"

	"github.com/albertocavalcante/msgraph-go/graphmock"
	teams "github.com/albertocavalcante/teams-go"
)

// A suite can exercise a bound heavily and never pin whether the boundary
// value itself is inside or outside it. Each test here fails if any of these
// clamps is off by one in either direction.

func TestMaxChatPageSizeIsInclusive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		top  int
		want int
	}{
		{"just under the cap", teams.MaxChatPageSize - 1, teams.MaxChatPageSize - 1},
		{"exactly the cap", teams.MaxChatPageSize, teams.MaxChatPageSize},
		{"one past the cap", teams.MaxChatPageSize + 1, teams.MaxChatPageSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := newTeamsServer(t)
			var gotTop int
			server.Handle("GET /chats", func(r *graphmock.Request) graphmock.Response {
				gotTop = r.Query.Top
				return graphmock.Collection([]teams.Chat{}, "")
			})
			client := newTeamsClient(t, server)

			for _, err := range client.Chats(context.Background(), teams.ChatQuery{Top: tc.top}) {
				if err != nil {
					t.Fatal(err)
				}
			}
			if gotTop != tc.want {
				t.Errorf("$top sent = %d, want %d", gotTop, tc.want)
			}
		})
	}
}

func TestMaxMessagePageSizeIsInclusive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		top  int
		want int
	}{
		{"just under the cap", teams.MaxMessagePageSize - 1, teams.MaxMessagePageSize - 1},
		{"exactly the cap", teams.MaxMessagePageSize, teams.MaxMessagePageSize},
		{"one past the cap", teams.MaxMessagePageSize + 1, teams.MaxMessagePageSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := newTeamsServer(t)
			var gotTop int
			server.Handle("GET /chats/{chat}/messages", func(r *graphmock.Request) graphmock.Response {
				gotTop = r.Query.Top
				return graphmock.Collection([]teams.ChatMessage{}, "")
			})
			client := newTeamsClient(t, server)

			for _, err := range client.Messages(context.Background(), "19:abc@thread.v2", teams.MessageQuery{Top: tc.top}) {
				if err != nil {
					t.Fatal(err)
				}
			}
			if gotTop != tc.want {
				t.Errorf("$top sent = %d, want %d", gotTop, tc.want)
			}
		})
	}
}

func TestMaxSearchPageSizeIsInclusive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		size int
		want int
	}{
		{"just under the cap", teams.MaxSearchPageSize - 1, teams.MaxSearchPageSize - 1},
		{"exactly the cap", teams.MaxSearchPageSize, teams.MaxSearchPageSize},
		{"one past the cap", teams.MaxSearchPageSize + 1, teams.MaxSearchPageSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := newTeamsServer(t)
			var gotSize int
			server.Handle("POST /search/query", func(r *graphmock.Request) graphmock.Response {
				body := graphmock.Decode[map[string]any](t, r)
				requests, _ := body["requests"].([]any)
				request, _ := requests[0].(map[string]any)
				gotSize = int(request["size"].(float64))
				return graphmock.Item(map[string]any{"value": []any{}})
			})
			client := newTeamsClient(t, server)

			if _, _, _, err := client.SearchPage(context.Background(), teams.SearchQuery{Term: "x", Size: tc.size}); err != nil {
				t.Fatal(err)
			}
			if gotSize != tc.want {
				t.Errorf("size sent = %d, want %d", gotSize, tc.want)
			}
		})
	}
}
