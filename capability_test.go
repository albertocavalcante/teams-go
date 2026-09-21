package teams_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	msgraph "github.com/albertocavalcante/msgraph-go"
	teams "github.com/albertocavalcante/teams-go"
)

func TestGranted(t *testing.T) {
	tests := []struct {
		name       string
		scopes     []string
		capability teams.Capability
		want       bool
	}{
		{"list chats with ChatReadBasic", []string{"Chat.ReadBasic"}, teams.ListChats, true},
		{"list chats with broader ChatReadWrite", []string{"Chat.ReadWrite"}, teams.ListChats, true},
		{"read chat messages needs Chat.Read", []string{"Chat.ReadBasic"}, teams.ReadChatMessages, false},
		{"read chat messages with Chat.Read", []string{"Chat.Read"}, teams.ReadChatMessages, true},
		{"send chat message with ChatMessage.Send", []string{"ChatMessage.Send"}, teams.SendChatMessage, true},
		{"send chat message with Chat.ReadWrite", []string{"Chat.ReadWrite"}, teams.SendChatMessage, true},
		// The split the whole package exists to respect: chat permissions
		// grant nothing in a channel.
		{"chat scopes do not cover channel messages", []string{"Chat.ReadWrite"}, teams.ReadChannelMessages, false},
		{"channel messages with ChannelMessage.Read.All", []string{"ChannelMessage.Read.All"}, teams.ReadChannelMessages, true},
		{"channel messages with Group.Read.All", []string{"Group.Read.All"}, teams.ReadChannelMessages, true},
		{"send channel message needs Send scope", []string{"ChannelMessage.Read.All"}, teams.SendChannelMessage, false},
		{"send channel message with ChannelMessage.Send", []string{"ChannelMessage.Send"}, teams.SendChannelMessage, true},
		{"own presence with Presence.Read", []string{"Presence.Read"}, teams.ReadOwnPresence, true},
		{"others' presence needs Presence.Read.All", []string{"Presence.Read"}, teams.ReadPresence, false},
		{"others' presence with Presence.Read.All", []string{"Presence.Read.All"}, teams.ReadPresence, true},
		// The service echoes scopes back with inconsistent casing.
		{"case insensitive", []string{"chat.readwrite"}, teams.ChangeChat, true},
		{"whitespace tolerated", []string{"  Chat.Read  "}, teams.ReadChatMessages, true},
		{"nothing granted", nil, teams.ListChats, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := teams.Granted(tt.scopes, tt.capability); got != tt.want {
				t.Fatalf("Granted = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMissingFor(t *testing.T) {
	granted := []string{"User.Read", "Chat.ReadWrite"}

	missing := teams.MissingFor(granted, teams.Capabilities()...)
	want := []teams.Capability{
		teams.ListTeams, teams.ListChannels, teams.ReadChannelMessages, teams.SendChannelMessage,
		teams.ReadOwnPresence, teams.ReadPresence,
	}
	if !slices.Equal(missing, want) {
		t.Fatalf("missing = %v, want %v", missing, want)
	}

	for _, capability := range []teams.Capability{
		teams.ReadIdentity, teams.ListChats, teams.ReadChatMessages,
		teams.SendChatMessage, teams.ChangeChat, teams.SearchMessages,
	} {
		if slices.Contains(missing, capability) {
			t.Errorf("%s should be covered by the granted scopes", capability)
		}
	}

	full := append(slices.Clone(granted),
		"Team.ReadBasic.All", "Channel.ReadBasic.All", "ChannelMessage.Read.All",
		"ChannelMessage.Send", "Presence.Read.All")
	if got := teams.MissingFor(full, teams.Capabilities()...); len(got) != 0 {
		t.Fatalf("missing = %v, want none", got)
	}
}

// The sign-in line must be stable, or every run suggests a different one.
func TestScopesToRequestIsStableAcross20Runs(t *testing.T) {
	granted := []string{"User.Read", "Chat.ReadWrite"}

	first := teams.ScopesToRequest(granted, teams.ListTeams, teams.ListChannels)
	for range 20 {
		if got := teams.ScopesToRequest(granted, teams.ListTeams, teams.ListChannels); !slices.Equal(got, first) {
			t.Fatalf("unstable: %v then %v", first, got)
		}
	}

	for _, scope := range granted {
		if !slices.Contains(first, scope) {
			t.Fatalf("scopes = %v, dropped %q", first, scope)
		}
	}
	if !slices.Contains(first, msgraph.ScopeTeamReadBasicAll) {
		t.Fatalf("scopes = %v, want the team permission added", first)
	}
}

func TestScopesToRequestDropsReservedScopes(t *testing.T) {
	granted := []string{"User.Read", "Chat.ReadWrite", "openid", "profile", "offline_access", "email"}

	got := teams.ScopesToRequest(granted, teams.ChangeChat)
	for _, reserved := range []string{"openid", "profile", "email", "offline_access"} {
		if slices.Contains(got, reserved) {
			t.Fatalf("scopes = %v, want %q filtered out", got, reserved)
		}
	}
	for _, scope := range []string{"User.Read", "Chat.ReadWrite"} {
		if !slices.Contains(got, scope) {
			t.Fatalf("scopes = %v, dropped %q", got, scope)
		}
	}
}

func TestIsReservedScope(t *testing.T) {
	for _, scope := range []string{"openid", "OpenID", " profile ", "offline_access", "email"} {
		if !teams.IsReservedScope(scope) {
			t.Errorf("IsReservedScope(%q) = false", scope)
		}
	}
	for _, scope := range []string{"Chat.Read", "User.Read", ""} {
		if teams.IsReservedScope(scope) {
			t.Errorf("IsReservedScope(%q) = true", scope)
		}
	}
}

func TestRequire(t *testing.T) {
	if err := teams.Require([]string{"Chat.ReadWrite"}, teams.ChangeChat); err != nil {
		t.Fatalf("Require = %v, want nil", err)
	}

	err := teams.Require([]string{"Chat.ReadWrite"}, teams.ListTeams)
	var missing *teams.ErrMissingCapability
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want ErrMissingCapability", err)
	}
	if missing.Capability != teams.ListTeams {
		t.Fatalf("capability = %q", missing.Capability)
	}
	if len(missing.Accepts) == 0 {
		t.Fatal("the error names no permission that would satisfy it")
	}
	if !missing.AdminOnly {
		t.Fatal("ListTeams needs admin consent; AdminOnly should be true")
	}
}

// Every capability must be answerable, or a caller enumerating them hits a
// silent gap.
func TestEveryCapabilityHasScopes(t *testing.T) {
	for _, capability := range teams.Capabilities() {
		if len(teams.ScopesFor(capability)) == 0 {
			t.Errorf("capability %q names no permission", capability)
		}
	}
	if len(teams.Capabilities()) == 0 {
		t.Fatal("no capabilities are declared")
	}
}

// Both the least and the most privileged scope for a capability must satisfy
// it on their own, because the identity service does not treat permissions
// as nested: holding the broad form does not make the narrow form
// obtainable, and a caller has to be able to try either.
func TestPrivilegeOrderIsDefined(t *testing.T) {
	t.Parallel()
	for _, capability := range teams.Capabilities() {
		least, okLeast := teams.LeastPrivilegedScope(capability)
		most, okMost := teams.MostPrivilegedScope(capability)
		if !okLeast || !okMost {
			t.Fatalf("%s has no scopes", capability)
		}
		if !teams.Granted([]string{least}, capability) {
			t.Errorf("%s: least-privileged %q does not satisfy it", capability, least)
		}
		if !teams.Granted([]string{most}, capability) {
			t.Errorf("%s: most-privileged %q does not satisfy it", capability, most)
		}
	}
}

// This is the split the whole package exists to make actionable: half of
// Teams' permissions cannot be granted by the signed-in user no matter how
// many times they sign in again.
func TestNeedsAdminConsent(t *testing.T) {
	t.Parallel()
	adminOnly := map[teams.Capability]bool{
		teams.ReadIdentity:        false,
		teams.ListChats:           false,
		teams.ReadChatMessages:    false,
		teams.SendChatMessage:     false,
		teams.ChangeChat:          false,
		teams.ListTeams:           true,
		teams.ListChannels:        true,
		teams.ReadChannelMessages: true,
		teams.SendChannelMessage:  false,
		teams.SearchMessages:      false,
		teams.ReadOwnPresence:     false,
		teams.ReadPresence:        true,
	}
	for _, capability := range teams.Capabilities() {
		want, ok := adminOnly[capability]
		if !ok {
			t.Fatalf("capability %q missing from the test table", capability)
		}
		if got := teams.NeedsAdminConsent(capability); got != want {
			t.Errorf("NeedsAdminConsent(%s) = %v, want %v", capability, got, want)
		}
	}
}

// TestEveryCapabilityScopeIsAKnownPermission closes the way
// [teams.NeedsAdminConsent] can answer without evidence.
//
// It derives the answer from msgraph's permission catalog, and a scope that is
// not in that catalog reads as not requiring admin consent -- the permissive
// direction, and the one that produces the loop the function exists to
// prevent. Pinned only by the table above, a capability repointed at an
// unregistered scope would keep passing while its answer became a default
// rather than a derivation.
func TestEveryCapabilityScopeIsAKnownPermission(t *testing.T) {
	t.Parallel()
	for _, capability := range teams.Capabilities() {
		for _, scope := range teams.ScopesFor(capability) {
			if teams.IsReservedScope(scope) {
				continue
			}
			if _, found := msgraph.FindPermission(scope); !found {
				t.Errorf("%s accepts %q, which msgraph does not know; NeedsAdminConsent would report false for it on no evidence",
					capability, scope)
			}
		}
	}
}

func TestUserConsentableExcludesAdminOnlyCapabilities(t *testing.T) {
	t.Parallel()
	consentable := teams.UserConsentable()
	if len(consentable) == 0 {
		t.Fatal("no capability is user-consentable")
	}
	for _, capability := range consentable {
		if teams.NeedsAdminConsent(capability) {
			t.Errorf("%s needs admin consent but was reported user-consentable", capability)
		}
	}
	for _, capability := range teams.Capabilities() {
		if teams.NeedsAdminConsent(capability) && slices.Contains(consentable, capability) {
			t.Errorf("%s needs admin consent but appears in UserConsentable", capability)
		}
	}
}

// The error text has to say something a caller can act on: asking a
// signed-in user to grant a permission they are never offered just loops.
func TestErrMissingCapabilityWording(t *testing.T) {
	t.Parallel()
	userErr := &teams.ErrMissingCapability{Capability: teams.ChangeChat, Accepts: []string{"Chat.ReadWrite"}}
	if got := userErr.Error(); !strings.Contains(got, "grant one of:") || strings.Contains(got, "administrator") {
		t.Errorf("user-consentable error = %q", got)
	}

	adminErr := &teams.ErrMissingCapability{Capability: teams.ListTeams, Accepts: []string{"Team.ReadBasic.All"}, AdminOnly: true}
	if got := adminErr.Error(); !strings.Contains(got, "a tenant administrator must grant one of:") {
		t.Errorf("admin-only error = %q, want it to name a tenant administrator", got)
	}
}
