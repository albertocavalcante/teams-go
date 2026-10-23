package teams_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	msgraph "github.com/albertocavalcante/msgraph-go"
	"github.com/albertocavalcante/msgraph-go/graphmock"
	teams "github.com/albertocavalcante/teams-go"
)

// TestMessageQueryBeforeFiltersOnTheSortedProperty checks that an exclusive
// upper bound and $orderby name the same field for both Order values. The
// fake -- carrying the real Teams constraints -- keeps the filter in both
// cases. The final act proves
// the constraint is genuinely live by hand-building a mismatched request
// straight through [teams.Client.Graph] and watching its filter get
// dropped, which is what [teams.MessageQuery] exists to make unreachable.
func TestMessageQueryBeforeFiltersOnTheSortedProperty(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, order := range []teams.MessageOrder{teams.ByCreated, teams.ByLastModified} {
		t.Run(string(order), func(t *testing.T) {
			t.Parallel()
			server := newTeamsServer(t)
			var sawFilter, sawOrderBy string
			server.Handle("GET /chats/{chat}/messages", func(r *graphmock.Request) graphmock.Response {
				sawFilter = r.Query.Filter
				sawOrderBy = strings.Join(r.Query.OrderBy, ",")
				return graphmock.Collection([]teams.ChatMessage{{ID: "1"}}, "")
			})
			client := newTeamsClient(t, server)

			query := teams.MessageQuery{Order: order, Before: since}
			for _, err := range client.Messages(context.Background(), "19:abc@thread.v2", query) {
				if err != nil {
					t.Fatal(err)
				}
			}

			field := string(order)
			if !strings.Contains(sawOrderBy, field) {
				t.Fatalf("orderby = %q, want it on %q", sawOrderBy, field)
			}
			if sawFilter == "" {
				t.Fatal("the fake dropped the filter -- MessageQuery must always keep filter and orderby on the same property")
			}
			if !strings.Contains(sawFilter, field) {
				t.Fatalf("filter = %q, want it on %q", sawFilter, field)
			}
		})
	}

	// Now the negative control: a hand-built request that names different
	// properties in its filter and its sort DOES get its filter dropped.
	// This is what proves the constraint is live, and what MessageQuery's
	// single Order field exists to make impossible to construct by accident.
	server := newTeamsServer(t)
	var sawFilter string
	server.Handle("GET /chats/{chat}/messages", func(r *graphmock.Request) graphmock.Response {
		sawFilter = r.Query.Filter
		return graphmock.Collection([]teams.ChatMessage{{ID: "1"}}, "")
	})
	client := newTeamsClient(t, server)

	_, _, err := msgraph.Get[msgraph.Page[teams.ChatMessage]](context.Background(), client.Graph(),
		"/chats/19:abc@thread.v2/messages", msgraph.Params{
			Where: msgraph.Ge(teams.FieldCreatedDateTime, since),
			Sort:  []msgraph.Order{msgraph.Desc(teams.FieldLastModifiedDateTime)},
		})
	if err != nil {
		t.Fatalf("a mismatched filter must be dropped, not refused: %v", err)
	}
	if sawFilter != "" {
		t.Fatalf("handler saw filter %q, want it dropped because the sort names a different property", sawFilter)
	}
}

func TestMessagesSinceFiltersLocallyAndStopsAtOlderMessage(t *testing.T) {
	t.Parallel()
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	server := newTeamsServer(t)
	requests := 0
	server.Handle("GET /chats/{chat}/messages", func(r *graphmock.Request) graphmock.Response {
		requests++
		if r.Query.Filter != "" {
			t.Errorf("since sent unsupported server filter: %q", r.Query.Filter)
		}
		return graphmock.Collection([]teams.ChatMessage{
			{ID: "newer", CreatedDateTime: cutoff.Add(time.Second)},
			{ID: "equal", CreatedDateTime: cutoff},
			{ID: "older", CreatedDateTime: cutoff.Add(-time.Second)},
		}, server.Link("/chats/c/messages", "page=2"))
	})
	client := newTeamsClient(t, server)
	var ids []teams.MessageID
	for message, err := range client.Messages(context.Background(), "c", teams.MessageQuery{Since: cutoff}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, message.ID)
	}
	if len(ids) != 2 || ids[0] != "newer" || ids[1] != "equal" {
		t.Fatalf("message IDs = %v, want [newer equal]", ids)
	}
	if requests != 1 {
		t.Fatalf("made %d requests, want to stop before older pages", requests)
	}
}

func TestListMessagesSinceFiltersPageAndEndsWindow(t *testing.T) {
	t.Parallel()
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	server := newTeamsServer(t)
	server.Handle("GET /chats/{chat}/messages", func(_ *graphmock.Request) graphmock.Response {
		return graphmock.Collection([]teams.ChatMessage{
			{ID: "equal", LastModifiedDateTime: cutoff},
			{ID: "older", LastModifiedDateTime: cutoff.Add(-time.Second)},
		}, server.Link("/chats/c/messages", "page=2"))
	})
	client := newTeamsClient(t, server)
	page, _, err := client.ListMessages(context.Background(), "c", teams.MessageQuery{Order: teams.ByLastModified, Since: cutoff})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Value) != 1 || page.Value[0].ID != "equal" || page.NextLink != "" {
		t.Fatalf("filtered page = %+v, want equal message and no continuation", page)
	}
}

func TestMessagesSinceFailsWhenTimestampIsMissing(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	server.Handle("GET /chats/{chat}/messages", func(_ *graphmock.Request) graphmock.Response {
		return graphmock.Collection([]teams.ChatMessage{{ID: "undated"}}, "")
	})
	client := newTeamsClient(t, server)
	var got error
	for _, err := range client.Messages(context.Background(), "c", teams.MessageQuery{Since: time.Now()}) {
		got = err
	}
	if !errors.Is(got, teams.ErrMissingMessageTimestamp) {
		t.Fatalf("error = %v, want ErrMissingMessageTimestamp", got)
	}
}

// TestMessageQueryRejectsAnInvertedWindow pins that a backwards window is
// caught before the request is built, because the service accepts it and
// answers an empty page -- which reads as a quiet chat, not a mistake.
func TestMessageQueryRejectsAnInvertedWindow(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	client := newTeamsClient(t, server)

	now := time.Now()
	query := teams.MessageQuery{Since: now, Before: now.Add(-time.Hour)}

	if err := query.Validate(); !errors.Is(err, teams.ErrInvertedWindow) {
		t.Fatalf("Validate() = %v, want ErrInvertedWindow", err)
	}

	var got error
	for _, err := range client.Messages(context.Background(), "19:abc@thread.v2", query) {
		got = err
	}
	if !errors.Is(got, teams.ErrInvertedWindow) {
		t.Fatalf("Messages() err = %v, want ErrInvertedWindow", got)
	}
	if len(server.Requests()) != 0 {
		t.Errorf("an inverted window must not reach the service, but %d requests did", len(server.Requests()))
	}
}

// TestAscendingIsNotExpressible pins that MessageQuery has no way to ask for
// ascending order: Params always renders a descending term, for every Order
// value, because the service refuses ascending and this type offers no
// field that could produce it.
func TestAscendingIsNotExpressible(t *testing.T) {
	t.Parallel()
	for _, order := range []teams.MessageOrder{teams.ByCreated, teams.ByLastModified, ""} {
		params := teams.MessageQuery{Order: order}.Params()
		if len(params.Sort) != 1 {
			t.Fatalf("order %q: Sort = %v, want exactly one term", order, params.Sort)
		}
		if !params.Sort[0].Descending {
			t.Fatalf("order %q rendered ascending; Params must always emit descending", order)
		}
	}
}
