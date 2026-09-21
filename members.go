package teams

import (
	"context"
	"iter"
	"time"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// MemberRole is a conversation member's role. It is a plain string rather
// than a closed enum for the same reason [ReactionType] is: the set is not
// exhaustively documented and an unrecognized value should still round-trip
// rather than collapse to nothing.
type MemberRole string

const (
	// MemberRoleOwner marks a member who can manage the chat or channel.
	MemberRoleOwner MemberRole = "owner"
	// MemberRoleGuest marks a member signed in from outside the tenant. The
	// absence of a role -- an empty Roles slice -- means an ordinary member,
	// not "role not loaded": member listings always populate Roles.
	MemberRoleGuest MemberRole = "guest"
)

// ConversationMember is one member of a chat or channel.
type ConversationMember struct {
	// ID identifies this membership record, not the user. It is an opaque
	// string scoped to the chat or channel; do not parse it. See
	// [ConversationMember.UserID] for the user's own identifier.
	ID          string       `json:"id,omitempty"`
	DisplayName string       `json:"displayName,omitempty"`
	UserID      UserID       `json:"userId,omitempty"`
	Email       string       `json:"email,omitempty"`
	Roles       []MemberRole `json:"roles,omitempty"`
	// VisibleHistoryStartDateTime is set when a member was added to an
	// existing chat and can see history only from that point forward. A nil
	// value means the member sees the whole history, not that this property
	// was not loaded -- member listings always populate it.
	VisibleHistoryStartDateTime *time.Time `json:"visibleHistoryStartDateTime,omitempty"`
}

// ChatMembers iterates a chat's members directly, following pagination.
//
// This exists because $expand=members on [Client.GetChat] or [Client.Chats]
// truncates at 25 members with no continuation link -- see
// [ChatQuery.WithMembers]. A chat reporting exactly 25 members from an
// expansion almost certainly has more; this endpoint is the only way to
// read them all. Member ids returned here are opaque strings scoped to the
// chat; do not parse them.
//
// Needs [ListChats].
func (c *Client) ChatMembers(ctx context.Context, chat ChatID) iter.Seq2[ConversationMember, error] {
	path, err := chatPath(chat)
	if err != nil {
		return failed[ConversationMember](err)
	}
	return items[ConversationMember](ctx, c, path+"/members", msgraph.Params{})
}

// ChannelMembers iterates a channel's members directly, following
// pagination, for the same reason and with the same truncation hazard as
// [Client.ChatMembers].
//
// Needs [ListChannels].
func (c *Client) ChannelMembers(ctx context.Context, team TeamID, channel ChannelID) iter.Seq2[ConversationMember, error] {
	path, err := teamChannelPath(team, channel)
	if err != nil {
		return failed[ConversationMember](err)
	}
	return items[ConversationMember](ctx, c, path+"/members", msgraph.Params{})
}
