package teams

import (
	"context"
	"errors"
	"fmt"
	"testing"

	msgraph "github.com/albertocavalcante/msgraph-go"
	"github.com/albertocavalcante/msgraph-go/graphmock"
)

func newTestClient(t *testing.T, server *graphmock.Server, opts ...Option) *Client {
	t.Helper()
	client, err := New(server.Client(), opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func TestPaginateFollowsNextLinkAcrossPages(t *testing.T) {
	t.Parallel()
	server := graphmock.New(t)
	pages := 0
	server.Handle("GET /messages", func(*graphmock.Request) graphmock.Response {
		pages++
		if pages >= 3 {
			return graphmock.Collection([]ChatMessage{{ID: MessageID(fmt.Sprint(pages))}}, "")
		}
		return graphmock.Collection([]ChatMessage{{ID: MessageID(fmt.Sprint(pages))}},
			server.Link("/messages", fmt.Sprintf("page=%d", pages)))
	})

	client := newTestClient(t, server)
	var got []MessageID
	for msg, err := range items[ChatMessage](context.Background(), client, "/messages", msgraph.Params{}) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, msg.ID)
	}
	if len(got) != 3 {
		t.Fatalf("got %d messages across 3 pages, want 3: %v", len(got), got)
	}
}

// Without a page count this pattern -- a server that mints a fresh nextLink
// every time -- runs forever, growing the seen-map by one URL per page the
// whole time. WithMaxPages is the only thing that stops it.
func TestPaginateStopsAtMaxPages(t *testing.T) {
	t.Parallel()
	server := graphmock.New(t)
	served := 0
	server.Handle("GET /messages", func(*graphmock.Request) graphmock.Response {
		served++
		return graphmock.Collection([]ChatMessage{{ID: MessageID(fmt.Sprint(served))}},
			server.Link("/messages", fmt.Sprintf("page=%d", served)))
	})

	client := newTestClient(t, server, WithMaxPages(5))
	var lastErr error
	count := 0
	for _, err := range items[ChatMessage](context.Background(), client, "/messages", msgraph.Params{}) {
		if err != nil {
			lastErr = err
			break
		}
		count++
	}
	if !errors.Is(lastErr, msgraph.ErrMaxPagesExceeded) {
		t.Fatalf("err = %v, want ErrMaxPagesExceeded", lastErr)
	}
	if served != 5 {
		t.Errorf("served %d pages, want the configured 5", served)
	}
}

// A server that repeats the exact same nextLink is a different failure than
// one that mints a fresh one every time, and is caught by the seen-map bound
// rather than the page-count bound.
func TestPaginateDetectsRepeatedNextLink(t *testing.T) {
	t.Parallel()
	server := graphmock.New(t)
	server.Handle("GET /messages", func(*graphmock.Request) graphmock.Response {
		return graphmock.Collection([]ChatMessage{{ID: "1"}}, server.Link("/messages", "same=always"))
	})

	client := newTestClient(t, server, WithMaxPages(1000))
	var lastErr error
	for _, err := range items[ChatMessage](context.Background(), client, "/messages", msgraph.Params{}) {
		if err != nil {
			lastErr = err
			break
		}
	}
	if !errors.Is(lastErr, msgraph.ErrPageCycle) {
		t.Fatalf("err = %v, want ErrPageCycle", lastErr)
	}
}

func TestFailedYieldsExactlyOneError(t *testing.T) {
	t.Parallel()
	wantErr := ErrEmptyID
	count := 0
	var got error
	for _, err := range failed[ChatMessage](wantErr) {
		count++
		got = err
	}
	if count != 1 {
		t.Fatalf("yielded %d times, want exactly 1", count)
	}
	if !errors.Is(got, wantErr) {
		t.Fatalf("err = %v, want %v", got, wantErr)
	}
}

func TestRequestCarriesClientPreferAndExtraDirectives(t *testing.T) {
	t.Parallel()
	server := graphmock.New(t)
	var gotPrefers []bool
	server.Handle("GET /messages", func(r *graphmock.Request) graphmock.Response {
		gotPrefers = append(gotPrefers,
			r.Prefers(msgraph.PreferIncludeUnknownEnumMembers),
			r.Prefers(msgraph.PreferIDTypeImmutableID))
		return graphmock.Collection([]ChatMessage{}, "")
	})

	client := newTestClient(t, server)
	req := client.request("GET", "/messages", msgraph.Params{}, nil, msgraph.PreferIDTypeImmutableID)
	if _, _, err := msgraph.Do[msgraph.Page[ChatMessage]](context.Background(), client.graph, req); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(gotPrefers) != 2 || !gotPrefers[0] || !gotPrefers[1] {
		t.Fatalf("prefers seen = %v, want both the mandatory and the extra directive", gotPrefers)
	}
}
