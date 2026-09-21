package teams

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"time"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// ChatType is Graph's evolvable chatType enum.
type ChatType string

const (
	ChatTypeOneOnOne           ChatType = "oneOnOne"
	ChatTypeGroup              ChatType = "group"
	ChatTypeMeeting            ChatType = "meeting"
	ChatTypeUnknownFutureValue ChatType = "unknownFutureValue"
)

// MaxChatPageSize is the largest page the chat listing serves, whatever
// $top asks for. A request above it is not refused; it is silently served
// this many.
const MaxChatPageSize = 50

// Chat is a one-on-one, group, or meeting chat.
type Chat struct {
	ID ChatID `json:"id,omitempty"`
	// Topic is null for a one-on-one chat: it has no name of its own. See
	// [Chat.DisplayName].
	Topic               string    `json:"topic,omitempty"`
	Type                ChatType  `json:"chatType,omitempty"`
	CreatedDateTime     time.Time `json:"createdDateTime,omitzero"`
	LastUpdatedDateTime time.Time `json:"lastUpdatedDateTime,omitzero"`
	WebURL              string    `json:"webUrl,omitempty"`
	TenantID            string    `json:"tenantId,omitempty"`
	// Viewpoint is the signed-in user's own read state, populated only when
	// requested. See [Chat.Unread].
	Viewpoint *ChatViewpoint `json:"viewpoint,omitempty"`
	// LastMessagePreview is populated only when [ChatQuery.WithLastMessagePreview]
	// or [ChatQuery.ByRecentMessage] was set.
	LastMessagePreview *MessagePreview `json:"lastMessagePreview,omitempty"`
	// Members is populated only when [ChatQuery.WithMembers] was set, and is
	// truncated at 25 with no continuation link; see [Client.ChatMembers]
	// for a chat that might have more.
	Members           []ConversationMember `json:"members,omitempty"`
	OnlineMeetingInfo *OnlineMeetingInfo   `json:"onlineMeetingInfo,omitempty"`
}

// ChatViewpoint is the signed-in user's own view of a chat: what nobody
// else's copy of the same chat resource would show.
type ChatViewpoint struct {
	IsHidden bool `json:"isHidden,omitempty"`
	// LastMessageReadDateTime is a pointer because "never opened this chat"
	// and "opened it at the zero time" are different facts. Comparing a
	// plain [time.Time] against a message's CreatedDateTime cannot tell them
	// apart; a nil here can.
	LastMessageReadDateTime *time.Time `json:"lastMessageReadDateTime,omitempty"`
}

// MessagePreview is the abbreviated message Graph attaches to a chat when
// [ChatQuery.WithLastMessagePreview] or [ChatQuery.ByRecentMessage] is set.
//
// It is a separate type from [ChatMessage] on purpose. The service
// populates only the handful of properties below; reusing ChatMessage here
// would present forty zero-valued fields -- Mentions, Reactions,
// Attachments, and the rest -- as though the message genuinely had none of
// them, rather than as "not part of a preview."
type MessagePreview struct {
	ID              MessageID    `json:"id,omitempty"`
	CreatedDateTime time.Time    `json:"createdDateTime,omitzero"`
	From            *IdentitySet `json:"from,omitempty"`
	Body            *ItemBody    `json:"body,omitempty"`
	MessageType     MessageType  `json:"messageType,omitempty"`
	IsDeleted       bool         `json:"isDeleted,omitempty"`
	EventDetail     *EventDetail `json:"eventDetail,omitempty"`
}

// OnlineMeetingInfo is the minimal join information Graph attaches to a
// meeting chat.
type OnlineMeetingInfo struct {
	Organizer  *IdentitySet `json:"organizer,omitempty"`
	JoinWebURL string       `json:"joinWebUrl,omitempty"`
}

// Unread reports whether the chat has a message newer than the signed-in
// user's last read mark.
//
// known is false when either half of that comparison was never asked for:
// LastMessagePreview was not expanded, or the chat was never opened (a nil
// [ChatViewpoint.LastMessageReadDateTime]). Reporting "read" in either case
// would mark every chat in a listing that simply did not request the
// preview as read, which is a worse answer than admitting the question
// cannot be answered from what was fetched.
func (c Chat) Unread() (unread, known bool) {
	if c.LastMessagePreview == nil || c.LastMessagePreview.CreatedDateTime.IsZero() {
		return false, false
	}
	if c.Viewpoint == nil || c.Viewpoint.LastMessageReadDateTime == nil {
		return false, false
	}
	return c.LastMessagePreview.CreatedDateTime.After(*c.Viewpoint.LastMessageReadDateTime), true
}

// DisplayName returns a name for the chat: Topic when the service sent one,
// otherwise -- for a one-on-one chat, whose Topic is always null -- the
// other participant's display name, found by excluding self from Members.
//
// Members is truncated at 25 with no continuation link (see
// [Client.ChatMembers]), which does not affect a one-on-one chat -- it only
// ever has two members -- but does mean this cannot reliably name "the
// other participants" for a large group chat with no topic of its own;
// it only ever looks at what [ChatQuery.WithMembers] already fetched.
func (c Chat) DisplayName(self UserID) string {
	if c.Topic != "" {
		return c.Topic
	}
	if c.Type != ChatTypeOneOnOne {
		return ""
	}
	for _, member := range c.Members {
		if member.UserID == self {
			continue
		}
		if member.DisplayName != "" {
			return member.DisplayName
		}
	}
	return ""
}

// ChatQuery selects and expands a chat listing.
//
// There is no Select, Where, or Sort field. The service serves exactly one
// ordering -- most recently updated, once ByRecentMessage asks for it -- and
// exactly two expansions, so those are booleans rather than an open-ended
// filter or sort a caller could aim at a property the service does not
// index.
type ChatQuery struct {
	// WithMembers expands the chat's members, truncated at 25 with no
	// continuation link. See [Client.ChatMembers] for a chat that might have
	// more.
	WithMembers bool
	// WithLastMessagePreview expands the chat's most recent message.
	WithLastMessagePreview bool
	// ByRecentMessage orders chats by their most recent message, newest
	// first, automatically adding the lastMessagePreview expansion -- see
	// [ChatQuery.Params] for why that addition is not optional.
	ByRecentMessage bool
	// Top asks for a page size, clamped to [MaxChatPageSize].
	Top int
	// Custom carries query parameters this package does not model.
	Custom url.Values
}

// Params renders the query as Graph query parameters.
//
// ByRecentMessage always adds the lastMessagePreview expansion, whether or
// not WithLastMessagePreview was also set. The service refuses to order a
// chat listing by a property inside an expansion it was not asked to
// perform -- ordering by [FieldLastMessagePreviewCreated] without expanding
// lastMessagePreview answers with an error the caller has no way to spell a
// request around, short of always remembering to set both fields together.
// Folding the expansion into ByRecentMessage removes that footgun: asking
// to sort by recent message is, by construction, also asking for the data
// the sort needs.
func (q ChatQuery) Params() msgraph.Params {
	var expand []string
	if q.WithMembers {
		expand = append(expand, "members")
	}
	if q.WithLastMessagePreview || q.ByRecentMessage {
		expand = append(expand, "lastMessagePreview")
	}
	params := msgraph.Params{
		Expand: expand,
		Top:    clampTop(q.Top, MaxChatPageSize),
		Custom: q.Custom,
	}
	if q.ByRecentMessage {
		params.Sort = []msgraph.Order{msgraph.Desc(FieldLastMessagePreviewCreated)}
	}
	return params
}

// chatPath addresses one chat.
func chatPath(chat ChatID) (string, error) {
	if strings.TrimSpace(chat.String()) == "" {
		return "", fmt.Errorf("%w: chat id", ErrEmptyID)
	}
	return "/chats/" + escapeID(chat.String()), nil
}

// Chats iterates the signed-in user's chats, following pagination.
//
// Needs [ListChats].
func (c *Client) Chats(ctx context.Context, query ChatQuery) iter.Seq2[Chat, error] {
	return items[Chat](ctx, c, "/chats", query.Params())
}

// ListChats returns one page of chats plus the [msgraph.Response] metadata.
// Use [Client.Chats] to page automatically.
//
// Needs [ListChats].
func (c *Client) ListChats(ctx context.Context, query ChatQuery) (msgraph.Page[Chat], *msgraph.Response, error) {
	return msgraph.Do[msgraph.Page[Chat]](ctx, c.graph,
		c.request(http.MethodGet, "/chats", query.Params(), nil))
}

// GetChat fetches a single chat.
//
// Needs [ListChats].
func (c *Client) GetChat(ctx context.Context, chat ChatID, query ChatQuery) (Chat, *msgraph.Response, error) {
	path, err := chatPath(chat)
	if err != nil {
		return Chat{}, nil, err
	}
	return msgraph.Do[Chat](ctx, c.graph, c.request(http.MethodGet, path, query.Params(), nil))
}
