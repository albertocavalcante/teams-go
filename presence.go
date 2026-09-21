package teams

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// Availability is a presence availability state.
//
// These values are Capitalized -- "Available", not "available" -- unlike
// every other enum in this package, all of which the service spells in
// camelCase. That is the service's own spelling for this one resource, not
// an inconsistency this package introduced, and comparing a decoded value
// against a lowercased literal shows up as "nobody is ever available,"
// silently, since the comparison simply never matches.
type Availability string

const (
	AvailabilityAvailable     Availability = "Available"
	AvailabilityAvailableIdle Availability = "AvailableIdle"
	AvailabilityAway          Availability = "Away"
	AvailabilityBeRightBack   Availability = "BeRightBack"
	AvailabilityBusy          Availability = "Busy"
	AvailabilityBusyIdle      Availability = "BusyIdle"
	AvailabilityDoNotDisturb  Availability = "DoNotDisturb"
	AvailabilityOffline       Availability = "Offline"
	AvailabilityUnknown       Availability = "PresenceUnknown"
)

// Activity is a presence activity, such as "InACall" or "Presenting". It is
// a plain string rather than a closed enum: the service documents this set
// as subject to change, and an unrecognized value should still round-trip
// rather than collapse to nothing.
type Activity string

// OutOfOfficeSettings is a user's out-of-office state, as reported by
// presence.
type OutOfOfficeSettings struct {
	IsOutOfOffice bool   `json:"isOutOfOffice,omitempty"`
	Message       string `json:"message,omitempty"`
}

// PresenceExpiry is Graph's dateTimeTimeZone shape, used by a presence
// status message's expiry. TimeZone is whatever zone name the service sent,
// which -- as elsewhere in Graph -- is not guaranteed to be an IANA name.
type PresenceExpiry struct {
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

// PresenceStatusMessage is a user-set status message shown alongside
// presence.
type PresenceStatusMessage struct {
	Message        *ItemBody       `json:"message,omitempty"`
	ExpiryDateTime *PresenceExpiry `json:"expiryDateTime,omitempty"`
}

// Presence is a user's Teams presence.
type Presence struct {
	ID            UserID                 `json:"id,omitempty"`
	Availability  Availability           `json:"availability,omitempty"`
	Activity      Activity               `json:"activity,omitempty"`
	OutOfOffice   *OutOfOfficeSettings   `json:"outOfOfficeSettings,omitempty"`
	StatusMessage *PresenceStatusMessage `json:"statusMessage,omitempty"`
}

// MyPresence reads the signed-in user's own presence.
//
// Needs [ReadOwnPresence].
func (c *Client) MyPresence(ctx context.Context) (Presence, *msgraph.Response, error) {
	return msgraph.Do[Presence](ctx, c.graph, c.request(http.MethodGet, "/me/presence", msgraph.Params{}, nil))
}

// UserPresence reads another user's presence.
//
// Needs [ReadPresence].
func (c *Client) UserPresence(ctx context.Context, user UserID) (Presence, *msgraph.Response, error) {
	if strings.TrimSpace(user.String()) == "" {
		return Presence{}, nil, fmt.Errorf("%w: user id", ErrEmptyID)
	}
	path := "/users/" + escapeID(user.String()) + "/presence"
	return msgraph.Do[Presence](ctx, c.graph, c.request(http.MethodGet, path, msgraph.Params{}, nil))
}
