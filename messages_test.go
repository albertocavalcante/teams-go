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

// TestMessageQueryAlwaysFiltersOnTheSortedProperty is the property the whole
// type exists to guarantee: $filter and $orderby always name the same
// field, for both Order values, and the fake -- carrying the real Teams
// constraints -- does not drop the filter for either. The final act proves
// the constraint is genuinely live by hand-building a mismatched request
// straight through [teams.Client.Graph] and watching its filter get
// dropped, which is what [teams.MessageQuery] exists to make unreachable.
func TestMessageQueryAlwaysFiltersOnTheSortedProperty(t *testing.T) {
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

			query := teams.MessageQuery{Order: order, Since: since}
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
