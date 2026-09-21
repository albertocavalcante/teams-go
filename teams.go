package teams

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"strings"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// TeamVisibility says who can find and join a team without an invitation.
type TeamVisibility string

const (
	TeamVisibilityPrivate          TeamVisibility = "private"
	TeamVisibilityPublic           TeamVisibility = "public"
	TeamVisibilityHiddenMembership TeamVisibility = "hiddenMembership"
)

// Team is a Microsoft Teams team.
//
// A field being its zero value on a value returned from [Client.JoinedTeams]
// does not mean the property is genuinely empty; see [Client.JoinedTeams].
type Team struct {
	ID          TeamID `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	// InternalID is the messaging thread id backing the team's General
	// channel conversation, not the team's own identifier. Passing it where
	// a TeamID is expected -- or a TeamID where this is expected -- 404s;
	// the two are different opaque strings that happen to share a shape.
	InternalID string         `json:"internalId,omitempty"`
	WebURL     string         `json:"webUrl,omitempty"`
	Visibility TeamVisibility `json:"visibility,omitempty"`
	IsArchived bool           `json:"isArchived,omitempty"`
	TenantID   string         `json:"tenantId,omitempty"`
}

// JoinedTeams iterates the teams the signed-in user belongs to.
//
// GET /me/joinedTeams accepts no OData query options at all -- no $select,
// $filter, $orderby, or $expand -- which is why this takes no query
// parameter, unlike every other listing in this package.
//
// The listing also populates only id, displayName, description, isArchived,
// and tenantId. Every other property, including Visibility, InternalID, and
// WebURL, comes back null. Reading Visibility off a value from this
// iterator sees the empty string and, if compared against
// [TeamVisibilityPublic] or read as "falsy", concludes the team is public --
// a different claim from "this property was never loaded." Call
// [Client.GetTeam] on the team's ID to read the rest.
//
// Needs [ListTeams].
func (c *Client) JoinedTeams(ctx context.Context) iter.Seq2[Team, error] {
	return items[Team](ctx, c, "/me/joinedTeams", msgraph.Params{})
}

// ListJoinedTeams returns one page of joined teams plus the
// [msgraph.Response] metadata. Use [Client.JoinedTeams] to page
// automatically. See [Client.JoinedTeams] for which properties are actually
// populated.
//
// Needs [ListTeams].
func (c *Client) ListJoinedTeams(ctx context.Context) (msgraph.Page[Team], *msgraph.Response, error) {
	return msgraph.Do[msgraph.Page[Team]](ctx, c.graph,
		c.request(http.MethodGet, "/me/joinedTeams", msgraph.Params{}, nil))
}

// GetTeam fetches a single team, populating every property -- including
// Visibility, InternalID, and WebURL, which a value from [Client.JoinedTeams]
// leaves null.
//
// Needs [ListTeams].
func (c *Client) GetTeam(ctx context.Context, team TeamID) (Team, *msgraph.Response, error) {
	if strings.TrimSpace(team.String()) == "" {
		return Team{}, nil, fmt.Errorf("%w: team id", ErrEmptyID)
	}
	return msgraph.Do[Team](ctx, c.graph,
		c.request(http.MethodGet, "/teams/"+escapeID(team.String()), msgraph.Params{}, nil))
}
