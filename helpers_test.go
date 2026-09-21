package teams_test

import (
	"testing"

	"github.com/albertocavalcante/msgraph-go/graphmock"
	teams "github.com/albertocavalcante/teams-go"
)

// newTeamsServer starts a fake Graph endpoint carrying the constraints and
// shapers a real Teams tenant applies -- see graphmock.Teams and
// graphmock.TeamsShapers. Using it instead of a bare graphmock.New means a
// request this package builds wrong fails here, at the speed of a unit
// test, instead of in production.
func newTeamsServer(t *testing.T) *graphmock.Server {
	t.Helper()
	return graphmock.New(t,
		graphmock.WithConstraints(graphmock.Teams()...),
		graphmock.WithShapers(graphmock.TeamsShapers()...),
	)
}

// newTeamsClient builds a [teams.Client] wired to server.
func newTeamsClient(t *testing.T, server *graphmock.Server, opts ...teams.Option) *teams.Client {
	t.Helper()
	client, err := teams.New(server.Client(), opts...)
	if err != nil {
		t.Fatalf("teams.New: %v", err)
	}
	return client
}
