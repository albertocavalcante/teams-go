package teams

import (
	"context"
	"testing"
	"time"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

func TestMessageIDTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		id     MessageID
		want   time.Time
		wantOK bool
	}{
		{"a real Graph message id", "1700000000000", time.UnixMilli(1700000000000), true},
		{"epoch zero is a valid id, not an error", "0", time.UnixMilli(0), true},
		{"garbage is not zero time", "not-a-number", time.Time{}, false},
		{"empty string", "", time.Time{}, false},
		{"negative is still numeric", "-5", time.UnixMilli(-5), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.id.Time()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && !got.Equal(tt.want) {
				t.Errorf("Time() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClampTop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		requested int
		max       int
		want      int
	}{
		{"zero requested stays zero", 0, 50, 0},
		{"just under the cap", 49, 50, 49},
		{"exactly the cap", 50, 50, 50},
		{"one past the cap", 51, 50, 50},
		{"far past the cap", 10000, 50, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampTop(tt.requested, tt.max); got != tt.want {
				t.Errorf("clampTop(%d, %d) = %d, want %d", tt.requested, tt.max, got, tt.want)
			}
		})
	}
}

func TestNewRejectsNilGraphClient(t *testing.T) {
	t.Parallel()
	_, err := New(nil)
	if err != ErrNilGraphClient {
		t.Fatalf("err = %v, want ErrNilGraphClient", err)
	}
}

// New must send msgraph.PreferIncludeUnknownEnumMembers on every request
// regardless of what the caller configured, since without it Teams
// substitutes unknownFutureValue for every system event message; see doc.go.
func TestNewSetsPreferDirective(t *testing.T) {
	t.Parallel()
	graph, err := msgraph.New(fakeTokenSource{})
	if err != nil {
		t.Fatalf("build graph client: %v", err)
	}

	client, err := New(graph)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !hasPrefer(client.prefer, msgraph.PreferIncludeUnknownEnumMembers) {
		t.Fatalf("prefer = %v, want it to include %q", client.prefer, msgraph.PreferIncludeUnknownEnumMembers)
	}

	// A caller's own directives must survive alongside the mandatory one.
	client, err = New(graph, WithPrefer(msgraph.PreferIDTypeImmutableID))
	if err != nil {
		t.Fatalf("New with WithPrefer: %v", err)
	}
	if !hasPrefer(client.prefer, msgraph.PreferIncludeUnknownEnumMembers) {
		t.Fatalf("prefer = %v, want the mandatory directive even alongside WithPrefer", client.prefer)
	}
	if !hasPrefer(client.prefer, msgraph.PreferIDTypeImmutableID) {
		t.Fatalf("prefer = %v, want the caller's directive preserved", client.prefer)
	}
}

func TestWithMaxPagesRejectsNonPositive(t *testing.T) {
	t.Parallel()
	graph, err := msgraph.New(fakeTokenSource{})
	if err != nil {
		t.Fatal(err)
	}
	for _, pages := range []int{0, -1, -100} {
		if _, err := New(graph, WithMaxPages(pages)); err == nil {
			t.Errorf("WithMaxPages(%d) was accepted", pages)
		}
	}
}

func TestSelfDefaultsToZeroValue(t *testing.T) {
	t.Parallel()
	graph, err := msgraph.New(fakeTokenSource{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(graph)
	if err != nil {
		t.Fatal(err)
	}
	if client.Self() != "" {
		t.Errorf("Self() = %q, want empty when WithSelf was not used", client.Self())
	}
	client, err = New(graph, WithSelf("u1"))
	if err != nil {
		t.Fatal(err)
	}
	if client.Self() != "u1" {
		t.Errorf("Self() = %q, want %q", client.Self(), "u1")
	}
}

func TestNotifyCapIsANoOpWithoutACallback(t *testing.T) {
	t.Parallel()
	graph, err := msgraph.New(fakeTokenSource{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(graph)
	if err != nil {
		t.Fatal(err)
	}
	// Must not panic.
	client.notifyCap(50, 25)
}

func TestNotifyCapInvokesTheConfiguredCallback(t *testing.T) {
	t.Parallel()
	graph, err := msgraph.New(fakeTokenSource{})
	if err != nil {
		t.Fatal(err)
	}
	var gotRequested, gotServed int
	calls := 0
	client, err := New(graph, WithPageCapNotify(func(requested, served int) {
		calls++
		gotRequested, gotServed = requested, served
	}))
	if err != nil {
		t.Fatal(err)
	}
	client.notifyCap(200, 50)
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if gotRequested != 200 || gotServed != 50 {
		t.Errorf("callback got (%d, %d), want (200, 50)", gotRequested, gotServed)
	}
}

func hasPrefer(directives []msgraph.PreferDirective, want msgraph.PreferDirective) bool {
	for _, d := range directives {
		if d == want {
			return true
		}
	}
	return false
}

type fakeTokenSource struct{}

func (fakeTokenSource) Token(_ context.Context) (string, error) { return "fake-token", nil }
