package teams_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/albertocavalcante/msgraph-go/graphmock"
	teams "github.com/albertocavalcante/teams-go"
)

// TestSearchPagesByOffsetNotNextLink pins that message search pages by
// advancing From by Size and watching moreResultsAvailable, since the
// endpoint carries no @odata.nextLink at all -- the one collection in this
// package that does not page the way every other one does.
func TestSearchPagesByOffsetNotNextLink(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	var froms []int
	server.Handle("POST /search/query", func(r *graphmock.Request) graphmock.Response {
		body := graphmock.Decode[map[string]any](t, r)
		requests, _ := body["requests"].([]any)
		if len(requests) != 1 {
			t.Fatalf("requests = %v, want exactly one", requests)
		}
		request, _ := requests[0].(map[string]any)
		from := int(request["from"].(float64))
		froms = append(froms, from)

		more := from == 0
		return graphmock.Item(map[string]any{
			"value": []any{
				map[string]any{
					"hitsContainers": []any{
						map[string]any{
							"hits": []any{
								map[string]any{
									"rank":     1,
									"summary":  "s",
									"resource": map[string]any{"id": fmt.Sprintf("m%d", from)},
								},
							},
							"total":                2,
							"moreResultsAvailable": more,
						},
					},
				},
			},
		})
	})
	client := newTeamsClient(t, server)

	var got []teams.MessageID
	for hit, err := range client.SearchMessages(context.Background(), teams.SearchQuery{Term: "budget", Size: 1}) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, hit.Message.ID)
	}
	if len(got) != 2 {
		t.Fatalf("got %d hits, want 2 spread across two offset pages: %v", len(got), got)
	}
	if len(froms) != 2 || froms[0] != 0 || froms[1] != 1 {
		t.Fatalf("from offsets sent = %v, want [0 1] -- paging must advance From by Size", froms)
	}
}
