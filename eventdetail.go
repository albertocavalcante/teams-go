package teams

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EventDetailType is the @odata.type of a system event message's eventDetail,
// naming which of the roughly forty variants Graph has added over the years
// it is.
type EventDetailType string

const (
	EventDetailMembersAdded      EventDetailType = "#microsoft.graph.membersAddedEventMessageDetail"
	EventDetailMembersDeleted    EventDetailType = "#microsoft.graph.membersDeletedEventMessageDetail"
	EventDetailMembersJoined     EventDetailType = "#microsoft.graph.membersJoinedEventMessageDetail"
	EventDetailMembersLeft       EventDetailType = "#microsoft.graph.membersLeftEventMessageDetail"
	EventDetailChatRenamed       EventDetailType = "#microsoft.graph.chatRenamedEventMessageDetail"
	EventDetailChannelAdded      EventDetailType = "#microsoft.graph.channelAddedEventMessageDetail"
	EventDetailChannelRenamed    EventDetailType = "#microsoft.graph.channelRenamedEventMessageDetail"
	EventDetailCallStarted       EventDetailType = "#microsoft.graph.callStartedEventMessageDetail"
	EventDetailCallEnded         EventDetailType = "#microsoft.graph.callEndedEventMessageDetail"
	EventDetailMeetingStarted    EventDetailType = "#microsoft.graph.meetingStartedEventMessageDetail"
	EventDetailMeetingEnded      EventDetailType = "#microsoft.graph.meetingEndedEventMessageDetail"
	EventDetailTeamsAppInstalled EventDetailType = "#microsoft.graph.teamsAppInstalledEventMessageDetail"
	EventDetailMessagePinned     EventDetailType = "#microsoft.graph.messagePinnedEventMessageDetail"
	EventDetailMessageUnpinned   EventDetailType = "#microsoft.graph.messageUnpinnedEventMessageDetail"
)

// knownEventDetailTypes is the set [EventDetail.Known] recognizes.
var knownEventDetailTypes = map[EventDetailType]bool{
	EventDetailMembersAdded:      true,
	EventDetailMembersDeleted:    true,
	EventDetailMembersJoined:     true,
	EventDetailMembersLeft:       true,
	EventDetailChatRenamed:       true,
	EventDetailChannelAdded:      true,
	EventDetailChannelRenamed:    true,
	EventDetailCallStarted:       true,
	EventDetailCallEnded:         true,
	EventDetailMeetingStarted:    true,
	EventDetailMeetingEnded:      true,
	EventDetailTeamsAppInstalled: true,
	EventDetailMessagePinned:     true,
	EventDetailMessageUnpinned:   true,
}

// EventDetail is a system event message's event payload.
//
// It is a discriminator plus a raw copy of the JSON rather than a Go
// interface with one implementation per variant. Graph has added roughly
// forty of these without a version bump, and an interface registry makes an
// unrecognized variant decode to nil -- which renders a message as a blank
// line where somebody left the team, was added to a call, or had a message
// pinned, with no indication anything was lost. Keeping the raw bytes means
// an unknown variant still round-trips and still answers [EventDetail.Decode]
// for whatever shape a caller already knows to expect from it.
type EventDetail struct {
	Type      EventDetailType `json:"@odata.type,omitempty"`
	Initiator *IdentitySet    `json:"initiator,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

// eventDetailAlias has EventDetail's fields without its Unmarshal/Marshal
// methods, so decoding through it does not recurse back into them.
type eventDetailAlias EventDetail

// UnmarshalJSON decodes the typed fields and keeps a copy of the raw bytes
// for [EventDetail.Decode] and for round-tripping an unrecognized variant.
//
// The copy is load-bearing: encoding/json reuses its scratch buffer across
// calls to a custom UnmarshalJSON, so data is only valid for the duration of
// this call, and keeping a reference to it instead of a copy would corrupt
// Raw the next time anything on the same decoder runs.
func (d *EventDetail) UnmarshalJSON(data []byte) error {
	raw := make(json.RawMessage, len(data))
	copy(raw, data)

	var alias eventDetailAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("teams: decode event detail: %w", err)
	}
	*d = EventDetail(alias)
	d.Raw = raw
	return nil
}

// MarshalJSON returns Raw when it is populated, so a value decoded from the
// service round-trips byte for byte including whatever variant-specific
// fields this type does not model. A value built by hand, with no Raw, falls
// back to marshaling the typed fields.
func (d EventDetail) MarshalJSON() ([]byte, error) {
	if len(d.Raw) > 0 {
		return d.Raw, nil
	}
	encoded, err := json.Marshal(eventDetailAlias(d))
	if err != nil {
		return nil, fmt.Errorf("teams: encode event detail: %w", err)
	}
	return encoded, nil
}

// Known reports whether Type is one of the variants this package recognizes.
// It is false for a nil receiver and for any @odata.type Graph has added
// since this package was last updated -- which [EventDetail.Decode] still
// answers, using Raw.
func (d *EventDetail) Known() bool {
	return d != nil && knownEventDetailTypes[d.Type]
}

// Decode unmarshals the raw event payload into into, giving access to a
// variant-specific field this type does not model directly.
func (d *EventDetail) Decode(into any) error {
	if d == nil {
		return fmt.Errorf("teams: decode into a nil event detail")
	}
	if len(d.Raw) == 0 {
		return fmt.Errorf("teams: event detail has no raw payload to decode")
	}
	if err := json.Unmarshal(d.Raw, into); err != nil {
		return fmt.Errorf("teams: decode event detail: %w", err)
	}
	return nil
}

// MembersAdded returns the members named by a membersAdded or membersJoined
// event, reporting false for any other event type.
func (d *EventDetail) MembersAdded() ([]Identity, bool) {
	if d == nil {
		return nil, false
	}
	switch d.Type {
	case EventDetailMembersAdded, EventDetailMembersJoined:
	default:
		return nil, false
	}
	var payload struct {
		Members []Identity `json:"members"`
	}
	if err := d.Decode(&payload); err != nil {
		return nil, false
	}
	return payload.Members, true
}

// MembersRemoved returns the members named by a membersDeleted or membersLeft
// event, reporting false for any other event type.
func (d *EventDetail) MembersRemoved() ([]Identity, bool) {
	if d == nil {
		return nil, false
	}
	switch d.Type {
	case EventDetailMembersDeleted, EventDetailMembersLeft:
	default:
		return nil, false
	}
	var payload struct {
		Members []Identity `json:"members"`
	}
	if err := d.Decode(&payload); err != nil {
		return nil, false
	}
	return payload.Members, true
}

// NewTopic returns the chat's new name from a chatRenamed event, reporting
// false for any other event type.
func (d *EventDetail) NewTopic() (string, bool) {
	if d == nil || d.Type != EventDetailChatRenamed {
		return "", false
	}
	var payload struct {
		ChatDisplayName string `json:"chatDisplayName"`
	}
	if err := d.Decode(&payload); err != nil {
		return "", false
	}
	return payload.ChatDisplayName, true
}

// CallDuration returns how long the call lasted from a callEnded event,
// reporting false for any other event type and for a duration this package
// cannot parse. A zero-length call and an unreadable one are different facts,
// and collapsing them both into a zero [time.Duration] would hide which one
// happened.
func (d *EventDetail) CallDuration() (time.Duration, bool) {
	if d == nil || d.Type != EventDetailCallEnded {
		return 0, false
	}
	var payload struct {
		CallDuration string `json:"callDuration"`
	}
	if err := d.Decode(&payload); err != nil {
		return 0, false
	}
	return parseISO8601Duration(payload.CallDuration)
}

// parseISO8601Duration parses the subset of ISO-8601 durations Graph uses for
// a call's length: PT#H#M#S, every component optional, seconds may carry a
// fractional part.
//
// [time.ParseDuration] does not read this format. Its "h"/"m"/"s" units look
// similar, but the leading "PT" makes it a different grammar, and handing it
// an ISO-8601 string fails with an error a caller has to remember to check --
// this exists so that check happens once, here, instead of at every call
// site.
func parseISO8601Duration(value string) (time.Duration, bool) {
	// A real call duration is nowhere near this long; the bound exists so a
	// hostile or corrupted value cannot make the loop below do more than a
	// trivial amount of work.
	const maxLen = 32
	if value == "" || len(value) > maxLen {
		return 0, false
	}
	rest, ok := strings.CutPrefix(value, "PT")
	if !ok {
		return 0, false
	}

	var total time.Duration
	var sawComponent bool
	for rest != "" {
		end := 0
		for end < len(rest) && (rest[end] == '.' || (rest[end] >= '0' && rest[end] <= '9')) {
			end++
		}
		if end == 0 || end >= len(rest) {
			return 0, false
		}
		amount, err := strconv.ParseFloat(rest[:end], 64)
		if err != nil {
			return 0, false
		}
		switch unit := rest[end]; unit {
		case 'H':
			total += time.Duration(amount * float64(time.Hour))
		case 'M':
			total += time.Duration(amount * float64(time.Minute))
		case 'S':
			total += time.Duration(amount * float64(time.Second))
		default:
			return 0, false
		}
		rest = rest[end+1:]
		sawComponent = true
	}
	if !sawComponent {
		return 0, false
	}
	return total, true
}

// EventSummary is a structured summary of a system event, built from
// whichever of [EventDetail]'s typed accessors apply to its Event type.
//
// This is a struct rather than an assembled sentence because wording, tense,
// and language are display decisions this package does not make on a
// caller's behalf.
type EventSummary struct {
	Event    EventDetailType
	Actor    Identity
	Subjects []Identity
	Topic    string
	Duration time.Duration
}

// Summary builds an [EventSummary] for a known event type, reporting false
// for a nil receiver or an event type [EventDetail.Known] does not recognize.
func (d *EventDetail) Summary() (EventSummary, bool) {
	if !d.Known() {
		return EventSummary{}, false
	}
	summary := EventSummary{Event: d.Type}
	if actor, _, ok := d.Initiator.Actor(); ok {
		summary.Actor = actor
	}
	switch d.Type {
	case EventDetailMembersAdded, EventDetailMembersJoined:
		summary.Subjects, _ = d.MembersAdded()
	case EventDetailMembersDeleted, EventDetailMembersLeft:
		summary.Subjects, _ = d.MembersRemoved()
	case EventDetailChatRenamed:
		summary.Topic, _ = d.NewTopic()
	case EventDetailCallEnded:
		summary.Duration, _ = d.CallDuration()
	}
	return summary, true
}
