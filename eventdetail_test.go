package teams_test

import (
	"encoding/json"
	"testing"
	"time"

	teams "github.com/albertocavalcante/teams-go"
)

// Raw has to round-trip byte for byte, because it is what keeps an
// unrecognized event variant from decoding to something silently empty.
func TestEventDetailRawRoundTrips(t *testing.T) {
	t.Parallel()
	original := []byte(`{"@odata.type":"#microsoft.graph.membersAddedEventMessageDetail","initiator":{"user":{"id":"u1","displayName":"Alice"}},"members":[{"id":"u2","displayName":"Bob"}]}`)

	var detail teams.EventDetail
	if err := json.Unmarshal(original, &detail); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	got, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var wantNorm, gotNorm any
	if err := json.Unmarshal(original, &wantNorm); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &gotNorm); err != nil {
		t.Fatal(err)
	}
	wantBytes, _ := json.Marshal(wantNorm)
	gotBytes, _ := json.Marshal(gotNorm)
	if string(wantBytes) != string(gotBytes) {
		t.Fatalf("round trip mismatch:\n got %s\nwant %s", gotBytes, wantBytes)
	}
}

// An unknown @odata.type is exactly the case Raw exists for: Known reports
// false, but the initiator still decoded because it is a typed field on
// EventDetail itself, and Decode still works because Raw was kept regardless
// of whether the type was recognized.
func TestEventDetailUnknownTypeStillDecodesInitiatorAndRaw(t *testing.T) {
	t.Parallel()
	data := []byte(`{"@odata.type":"#microsoft.graph.someFutureEventMessageDetail","initiator":{"user":{"id":"u1","displayName":"Alice"}},"newField":"surprise"}`)

	var detail teams.EventDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if detail.Known() {
		t.Error("Known() = true for an unrecognized @odata.type")
	}
	if detail.Initiator == nil {
		t.Fatal("Initiator = nil, want the typed field to have decoded")
	}
	actor, _, ok := detail.Initiator.Actor()
	if !ok || actor.DisplayName != "Alice" {
		t.Errorf("Initiator actor = %+v, ok=%v, want Alice", actor, ok)
	}

	var payload struct {
		NewField string `json:"newField"`
	}
	if err := detail.Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if payload.NewField != "surprise" {
		t.Errorf("NewField = %q, want %q", payload.NewField, "surprise")
	}
}

func TestEventDetailNilReceiverIsSafe(t *testing.T) {
	t.Parallel()
	var detail *teams.EventDetail
	if detail.Known() {
		t.Error("Known() on nil receiver = true")
	}
	if err := detail.Decode(&struct{}{}); err == nil {
		t.Error("Decode on nil receiver = nil error, want one")
	}
	if _, ok := detail.MembersAdded(); ok {
		t.Error("MembersAdded on nil receiver = true")
	}
	if _, ok := detail.CallDuration(); ok {
		t.Error("CallDuration on nil receiver = true")
	}
	if _, ok := detail.Summary(); ok {
		t.Error("Summary on nil receiver = true")
	}
}

func TestEventDetailMembersAddedAndRemoved(t *testing.T) {
	t.Parallel()
	added := []byte(`{"@odata.type":"#microsoft.graph.membersAddedEventMessageDetail","members":[{"id":"u1","displayName":"Alice"},{"id":"u2","displayName":"Bob"}]}`)
	var addedDetail teams.EventDetail
	if err := json.Unmarshal(added, &addedDetail); err != nil {
		t.Fatal(err)
	}
	members, ok := addedDetail.MembersAdded()
	if !ok || len(members) != 2 || members[0].DisplayName != "Alice" {
		t.Errorf("MembersAdded = %+v, ok=%v", members, ok)
	}
	if _, ok := addedDetail.MembersRemoved(); ok {
		t.Error("MembersRemoved on a membersAdded detail = true")
	}

	left := []byte(`{"@odata.type":"#microsoft.graph.membersLeftEventMessageDetail","members":[{"id":"u3","displayName":"Carol"}]}`)
	var leftDetail teams.EventDetail
	if err := json.Unmarshal(left, &leftDetail); err != nil {
		t.Fatal(err)
	}
	removed, ok := leftDetail.MembersRemoved()
	if !ok || len(removed) != 1 || removed[0].DisplayName != "Carol" {
		t.Errorf("MembersRemoved = %+v, ok=%v", removed, ok)
	}
}

func TestEventDetailNewTopic(t *testing.T) {
	t.Parallel()
	data := []byte(`{"@odata.type":"#microsoft.graph.chatRenamedEventMessageDetail","chatDisplayName":"Project Falcon"}`)
	var detail teams.EventDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		t.Fatal(err)
	}
	topic, ok := detail.NewTopic()
	if !ok || topic != "Project Falcon" {
		t.Errorf("NewTopic = %q, ok=%v", topic, ok)
	}
}

// A zero-length call and an unparseable duration are different facts:
// CallDuration must report false for the latter, not a silent zero that
// reads the same as the former.
func TestEventDetailCallDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		duration string
		want     time.Duration
		wantOK   bool
	}{
		{"hours minutes seconds", "PT1H30M15S", time.Hour + 30*time.Minute + 15*time.Second, true},
		{"minutes only", "PT45M", 45 * time.Minute, true},
		{"zero length call", "PT0S", 0, true},
		{"fractional seconds", "PT1.5S", 1500 * time.Millisecond, true},
		{"unparseable garbage", "not-a-duration", 0, false},
		{"empty string", "", 0, false},
		{"missing PT prefix", "1H30M", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]string{
				"@odata.type":  string(teams.EventDetailCallEnded),
				"callDuration": tt.duration,
			})
			if err != nil {
				t.Fatal(err)
			}
			var detail teams.EventDetail
			if err := json.Unmarshal(data, &detail); err != nil {
				t.Fatal(err)
			}
			got, ok := detail.CallDuration()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("duration = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventDetailSummary(t *testing.T) {
	t.Parallel()
	data := []byte(`{"@odata.type":"#microsoft.graph.membersAddedEventMessageDetail","initiator":{"user":{"id":"u1","displayName":"Alice"}},"members":[{"id":"u2","displayName":"Bob"}]}`)
	var detail teams.EventDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		t.Fatal(err)
	}
	summary, ok := detail.Summary()
	if !ok {
		t.Fatal("Summary() ok = false for a known event")
	}
	if summary.Event != teams.EventDetailMembersAdded {
		t.Errorf("Event = %q", summary.Event)
	}
	if summary.Actor.DisplayName != "Alice" {
		t.Errorf("Actor = %+v", summary.Actor)
	}
	if len(summary.Subjects) != 1 || summary.Subjects[0].DisplayName != "Bob" {
		t.Errorf("Subjects = %+v", summary.Subjects)
	}
}
