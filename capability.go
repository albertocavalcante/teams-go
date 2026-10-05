package teams

import (
	"fmt"
	"slices"
	"strings"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// Capability is something this client can be asked to do.
//
// Every operation in this package documents the capability it needs, and the
// capability names the delegated permissions that satisfy it. This exists
// because the service is unhelpful about permissions: a request made without
// the right one is refused with a bare error that names nothing, so there is
// no way to learn from the failure what to ask for. Checking beforehand is
// the only way to give an answer.
type Capability string

const (
	// ReadIdentity reads the signed-in user's own profile.
	ReadIdentity Capability = "read your profile"
	// ListChats lists chats and their members.
	ListChats Capability = "list chats"
	// ReadChatMessages reads messages in a chat.
	ReadChatMessages Capability = "read chat messages"
	// SendChatMessage sends a chat message.
	SendChatMessage Capability = "send a chat message"
	// ChangeChat reacts to or soft-deletes a chat message.
	ChangeChat Capability = "change chat messages"
	// ListTeams lists the teams the signed-in user belongs to.
	ListTeams Capability = "list your teams"
	// ListChannels lists a team's channels.
	ListChannels Capability = "list channels"
	// ReadChannelMessages reads channel messages and their replies.
	ReadChannelMessages Capability = "read channel messages"
	// SendChannelMessage posts to a channel.
	SendChannelMessage Capability = "send a channel message"
	// SearchMessages runs a full-text message search.
	SearchMessages Capability = "search messages"
	// ReadOwnPresence reads the signed-in user's own presence.
	ReadOwnPresence Capability = "read your presence"
	// ReadPresence reads another user's presence.
	ReadPresence Capability = "read others' presence"
)

// satisfiedBy lists the delegated permissions that satisfy each capability,
// narrowest first. Any one of them is enough, which is what lets a broader
// permission stand in for a narrower one.
var satisfiedBy = map[Capability][]string{
	ReadIdentity: {msgraph.ScopeUserRead},

	ListChats:        {msgraph.ScopeChatReadBasic, msgraph.ScopeChatRead, msgraph.ScopeChatReadWrite},
	ReadChatMessages: {msgraph.ScopeChatRead, msgraph.ScopeChatReadWrite},
	SendChatMessage:  {msgraph.ScopeChatMessageSend, msgraph.ScopeChatReadWrite},
	ChangeChat:       {msgraph.ScopeChatReadWrite},

	ListTeams:           {msgraph.ScopeTeamReadBasicAll, msgraph.ScopeDirectoryReadAll},
	ListChannels:        {msgraph.ScopeChannelReadBasicAll, msgraph.ScopeGroupReadAll, msgraph.ScopeGroupReadWriteAll, msgraph.ScopeDirectoryReadAll},
	ReadChannelMessages: {msgraph.ScopeChannelMessageReadAll, msgraph.ScopeGroupReadAll, msgraph.ScopeGroupReadWriteAll},
	SendChannelMessage:  {msgraph.ScopeChannelMessageSend, msgraph.ScopeGroupReadWriteAll},

	SearchMessages: {msgraph.ScopeChatRead, msgraph.ScopeChatReadWrite},

	ReadOwnPresence: {msgraph.ScopePresenceRead, msgraph.ScopePresenceReadAll},
	ReadPresence:    {msgraph.ScopePresenceReadAll},
}

// order fixes the reporting order, so a caller enumerating capabilities gets
// the same answer every time rather than a map's iteration order.
var order = []Capability{
	ReadIdentity,
	ListChats,
	ReadChatMessages,
	SendChatMessage,
	ChangeChat,
	ListTeams,
	ListChannels,
	ReadChannelMessages,
	SendChannelMessage,
	SearchMessages,
	ReadOwnPresence,
	ReadPresence,
}

// Capabilities lists every capability, narrowest first.
func Capabilities() []Capability { return slices.Clone(order) }

// ScopesFor returns the delegated permissions that satisfy a capability. Any
// one of them is sufficient; the first is the least privileged.
func ScopesFor(capability Capability) []string {
	return slices.Clone(satisfiedBy[capability])
}

// LeastPrivilegedScope returns the narrowest permission that satisfies a
// capability, which is what to ask for when adding one.
func LeastPrivilegedScope(capability Capability) (string, bool) {
	scopes := satisfiedBy[capability]
	if len(scopes) == 0 {
		return "", false
	}
	return scopes[0], true
}

// MostPrivilegedScope returns the broadest permission that satisfies a
// capability.
//
// It exists because the identity service does not treat permissions as
// nested. Holding Chat.ReadWrite does not make Chat.Read obtainable: they are
// unrelated strings, and asking for one the user never consented to is
// refused outright. A caller that asked for the least privileged scope and
// was refused should try this before concluding the capability is
// unavailable, since consenting by capability commonly grants only the
// broader form.
func MostPrivilegedScope(capability Capability) (string, bool) {
	scopes := satisfiedBy[capability]
	if len(scopes) == 0 {
		return "", false
	}
	return scopes[len(scopes)-1], true
}

// Granted reports whether the given scopes satisfy a capability.
//
// This delegates to [msgraph.ScopeSatisfies] rather than a plain membership
// check, which is what lets a broad grant such as Group.ReadWrite.All stand
// in for [ReadChannelMessages] without this package having to repeat the
// nesting msgraph-go already maintains. Comparison is case-insensitive
// because the service echoes granted scopes back with inconsistent casing.
func Granted(scopes []string, capability Capability) bool {
	for _, accepted := range satisfiedBy[capability] {
		if msgraph.ScopeSatisfies(scopes, accepted) {
			return true
		}
	}
	return false
}

// MissingFor returns the capabilities the given scopes do not satisfy, in a
// stable order.
func MissingFor(scopes []string, capabilities ...Capability) []Capability {
	var missing []Capability
	for _, capability := range capabilities {
		if !Granted(scopes, capability) {
			missing = append(missing, capability)
		}
	}
	return missing
}

// ScopesToRequest returns the scopes to ask for so that the given
// capabilities are all satisfied, keeping any already granted and adding the
// least privileged permission for each that is not.
//
// The result is deduplicated and stably ordered, so re-running it produces
// the same sign-in line rather than a reshuffled one. The granted argument
// must be what consent actually covers, not the scopes a client is
// configured with. Passing the configuration instead makes this add the
// least-privileged scope for a capability the user has already granted in
// its broader form -- a permission that was never consented to, which the
// identity service refuses.
func ScopesToRequest(granted []string, capabilities ...Capability) []string {
	result := make([]string, 0, len(granted)+len(capabilities))
	seen := map[string]bool{}
	add := func(scope string) {
		key := strings.ToLower(scope)
		if scope == "" || seen[key] {
			return
		}
		seen[key] = true
		result = append(result, scope)
	}

	for _, scope := range granted {
		scope = strings.TrimSpace(scope)
		if IsReservedScope(scope) {
			continue
		}
		add(scope)
	}
	for _, capability := range capabilities {
		if Granted(result, capability) {
			continue
		}
		if scope, ok := LeastPrivilegedScope(capability); ok {
			add(scope)
		}
	}
	return result
}

// reservedScopes are granted implicitly by the identity platform and appear
// in what the service reports back. They are filtered out of any sign-in
// line, because asking for them explicitly is noise at best: the library
// adds them itself, and echoing them back suggests they are something the
// user chose.
var reservedScopes = map[string]bool{
	"openid":         true,
	"profile":        true,
	"email":          true,
	"offline_access": true,
}

// IsReservedScope reports whether a scope is granted implicitly rather than
// requested.
func IsReservedScope(scope string) bool {
	return reservedScopes[strings.ToLower(strings.TrimSpace(scope))]
}

// NeedsAdminConsent reports whether the least privileged scope that satisfies
// a capability can only be granted by a tenant administrator.
//
// Every Outlook mail permission is user-consentable: a caller missing one can
// always fix it by signing in again and accepting a consent screen. Half of
// the Teams permissions are not -- Team.ReadBasic.All, Channel.ReadBasic.All,
// ChannelMessage.Read.All, and Presence.Read.All are tenant-wide by
// construction, and a signed-in user is never offered them regardless of how
// many times they retry sign-in. Telling a caller to grant one is advice they
// cannot act on, and a caller that treats every missing capability the same
// way loops forever against a consent screen that will never offer it.
func NeedsAdminConsent(capability Capability) bool {
	scope, ok := LeastPrivilegedScope(capability)
	if !ok {
		return false
	}
	permission, found := msgraph.FindPermission(scope)
	return found && permission.AdminConsentRequired
}

// UserConsentable returns the capabilities a signed-in user can grant by
// consenting, in the stable [Capabilities] order. It excludes anything
// [NeedsAdminConsent] reports true for.
func UserConsentable() []Capability {
	consentable := make([]Capability, 0, len(order))
	for _, capability := range order {
		if !NeedsAdminConsent(capability) {
			consentable = append(consentable, capability)
		}
	}
	return consentable
}

// ErrMissingCapability reports a permission the caller does not hold. It is
// returned before the request is sent, so nothing is attempted that the
// service would only refuse.
type ErrMissingCapability struct {
	// Capability is what was needed.
	Capability Capability
	// Accepts lists the permissions that would satisfy it.
	Accepts []string
	// AdminOnly is true when none of Accepts can be granted by the
	// signed-in user; see [NeedsAdminConsent].
	AdminOnly bool
}

func (e *ErrMissingCapability) Error() string {
	if e.AdminOnly {
		return fmt.Sprintf("teams: not permitted to %s; a tenant administrator must grant one of: %s",
			e.Capability, strings.Join(e.Accepts, ", "))
	}
	return fmt.Sprintf("teams: not permitted to %s; grant one of: %s",
		e.Capability, strings.Join(e.Accepts, ", "))
}

// Require reports an error when the given scopes do not satisfy a
// capability. It is the check to make before a request rather than after a
// refusal.
func Require(scopes []string, capability Capability) error {
	if Granted(scopes, capability) {
		return nil
	}
	return &ErrMissingCapability{
		Capability: capability,
		Accepts:    ScopesFor(capability),
		AdminOnly:  NeedsAdminConsent(capability),
	}
}
