package teams_test

import (
	"context"
	"testing"

	"github.com/albertocavalcante/msgraph-go/graphmock"
	teams "github.com/albertocavalcante/teams-go"
)

// TestPresenceValuesAreCapitalized pins that Availability decodes the
// service's Capitalized spelling -- "Available", not "available" -- which is
// the one enum in this package the service does not spell in camelCase.
// Comparing a decoded value against a lowercased constant would compile,
// never match, and read as "nobody is ever available."
func TestPresenceValuesAreCapitalized(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	server.Handle("GET /me/presence", func(*graphmock.Request) graphmock.Response {
		return graphmock.Item(map[string]any{
			"id":           "me",
			"availability": "Available",
			"activity":     "Available",
		})
	})
	client := newTeamsClient(t, server)

	presence, _, err := client.MyPresence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if presence.Availability != teams.AvailabilityAvailable {
		t.Fatalf("Availability = %q, want %q", presence.Availability, teams.AvailabilityAvailable)
	}
	// Negative control: without this, a bug that decoded into a lowercased
	// constant would still pass the assertion above by coincidence of both
	// sides being wrong the same way.
	if teams.Availability("available") == teams.AvailabilityAvailable {
		t.Fatal("a lowercased literal must not equal the service's Capitalized spelling")
	}
}
