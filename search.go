package teams

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strings"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// MaxSearchPageSize is the largest page a message search serves, whatever
// Size asks for.
const MaxSearchPageSize = 25

// ErrEmptySearchTerm means a [SearchQuery] carries no search term. The
// service accepts an empty queryString and answers with a search over
// nothing that reads as "no matches" rather than as a malformed request.
var ErrEmptySearchTerm = errors.New("teams: search term is empty")

// SearchQuery is a full-text message search.
//
// There is no sort field: the endpoint refuses sortProperties for a
// chatMessage search outright, answering 400. Message hits always come
// back in relevance order, and a caller wanting them chronological has to
// sort the hits itself after fetching them.
type SearchQuery struct {
	// Term is the text to search for.
	Term string
	// From is the zero-based offset of the first hit to return.
	From int
	// Size is the page size, clamped to [MaxSearchPageSize]. Zero, like a
	// negative value, is treated as [MaxSearchPageSize] rather than as "ask
	// the service for its own default": unlike an OData $top, this number
	// also has to be added to From between pages, and a page size this
	// package never learned cannot be added correctly.
	Size int
	// Fields restricts which properties of each hit's resource the service
	// returns.
	Fields []string
}

// SearchHit is one match from a message search.
type SearchHit struct {
	Rank    int32       `json:"rank"`
	Summary string      `json:"summary,omitempty"`
	Message ChatMessage `json:"resource"`
}

// searchRequestBody is the body POST /search/query expects.
type searchRequestBody struct {
	Requests []searchRequest `json:"requests"`
}

type searchRequest struct {
	EntityTypes []string           `json:"entityTypes"`
	Query       searchRequestQuery `json:"query"`
	From        int                `json:"from"`
	Size        int                `json:"size"`
	Fields      []string           `json:"fields,omitempty"`
}

type searchRequestQuery struct {
	QueryString string `json:"queryString"`
}

// searchResponseBody is the shape of a search/query response, narrowed to
// the one request this package ever sends and the one hit container it
// produces.
type searchResponseBody struct {
	Value []struct {
		HitsContainers []struct {
			Hits                 []SearchHit `json:"hits"`
			Total                int64       `json:"total"`
			MoreResultsAvailable bool        `json:"moreResultsAvailable"`
		} `json:"hitsContainers"`
	} `json:"value"`
}

// searchPageSize resolves the page size to send and to advance From by. See
// [SearchQuery.Size] for why zero does not mean "ask the service."
func searchPageSize(requested int) int {
	if requested <= 0 || requested > MaxSearchPageSize {
		return MaxSearchPageSize
	}
	return requested
}

// SearchPage runs one page of a message search.
//
// With Chat.Read alone -- and no channel-message permission -- this returns
// chat hits and no channel hits, with no error and nothing in the response
// distinguishing that from a search that genuinely found nothing in
// channels. An answer that looks complete is not the same as one that
// searched everywhere it could have.
//
// Needs [SearchMessages].
func (c *Client) SearchPage(ctx context.Context, query SearchQuery) (hits []SearchHit, more bool, resp *msgraph.Response, err error) {
	if strings.TrimSpace(query.Term) == "" {
		return nil, false, nil, ErrEmptySearchTerm
	}
	body := searchRequestBody{Requests: []searchRequest{{
		EntityTypes: []string{"chatMessage"},
		Query:       searchRequestQuery{QueryString: query.Term},
		From:        query.From,
		Size:        searchPageSize(query.Size),
		Fields:      query.Fields,
	}}}
	result, response, err := msgraph.Do[searchResponseBody](ctx, c.graph,
		c.request(http.MethodPost, "/search/query", msgraph.Params{}, body))
	if err != nil {
		return nil, false, response, err
	}
	if len(result.Value) == 0 || len(result.Value[0].HitsContainers) == 0 {
		return nil, false, response, nil
	}
	container := result.Value[0].HitsContainers[0]
	return container.Hits, container.MoreResultsAvailable, response, nil
}

// SearchMessages runs a full-text message search, following the
// service's offset-based continuation until moreResultsAvailable is false.
//
// This is the one collection in this package with no @odata.nextLink: the
// endpoint pages a search by advancing From by the page size instead, and
// nothing in [Client.Graph] or the foundation's paginate helper models
// that. A run is bounded by the client's [WithMaxPages] the same way every
// other listing here is, so a service that never stops reporting more
// results available cannot make this loop forever.
//
// Needs [SearchMessages].
func (c *Client) SearchMessages(ctx context.Context, query SearchQuery) iter.Seq2[SearchHit, error] {
	return func(yield func(SearchHit, error) bool) {
		if strings.TrimSpace(query.Term) == "" {
			yield(SearchHit{}, ErrEmptySearchTerm)
			return
		}
		size := searchPageSize(query.Size)
		from := query.From
		maxPages := c.maxPages
		if maxPages <= 0 {
			maxPages = msgraph.DefaultMaxPages
		}

		for range maxPages {
			hits, more, _, err := c.SearchPage(ctx, SearchQuery{
				Term: query.Term, From: from, Size: size, Fields: query.Fields,
			})
			if err != nil {
				yield(SearchHit{}, err)
				return
			}
			for i := range hits {
				if !yield(hits[i], nil) {
					return
				}
			}
			if !more {
				return
			}
			from += size
		}
		yield(SearchHit{}, fmt.Errorf("%w: stopped after %d pages", msgraph.ErrMaxPagesExceeded, maxPages))
	}
}
