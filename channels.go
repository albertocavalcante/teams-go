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

// ChannelMembership is Graph's evolvable channelMembershipType enum.
type ChannelMembership string

const (
	ChannelStandard           ChannelMembership = "standard"
	ChannelPrivate            ChannelMembership = "private"
	ChannelShared             ChannelMembership = "shared"
	ChannelUnknownFutureValue ChannelMembership = "unknownFutureValue"
)

// Channel is a channel within a team.
//
// Membership matters beyond description: a private or shared channel keeps
// its messages on a back end the team's own message endpoints do not reach.
// [Client.ChannelMessages] against one of these returns an empty listing,
// not a permission error, which reads as "no messages yet" rather than "you
// are looking in the wrong place."
type Channel struct {
	ID          ChannelID `json:"id,omitempty"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	// Email is expensive for the service to populate. See [ChannelQuery.Select].
	Email               string            `json:"email,omitempty"`
	WebURL              string            `json:"webUrl,omitempty"`
	Membership          ChannelMembership `json:"membershipType,omitempty"`
	CreatedDateTime     time.Time         `json:"createdDateTime,omitzero"`
	IsArchived          bool              `json:"isArchived,omitempty"`
	IsFavoriteByDefault bool              `json:"isFavoriteByDefault,omitempty"`
	TenantID            string            `json:"tenantId,omitempty"`
}

// ChannelQuery selects and filters a channel listing.
//
// Unlike a chat or channel message listing, a channel listing genuinely
// supports $filter and $select, which is why -- unlike [MessageQuery] and
// [ChatQuery] -- this has a Select and a Where field instead of a closed
// set of booleans.
type ChannelQuery struct {
	// Select limits the properties Graph returns. Excluding "email" is worth
	// doing deliberately: the service computes it from the channel's
	// underlying group mailbox, which costs measurably more than the rest of
	// the channel's properties combined, and a listing rendering only names
	// and descriptions does not need to pay for it.
	Select []string
	// Where is a typed $filter.
	Where msgraph.Expr
	// Custom carries query parameters this package does not model.
	Custom url.Values
}

// Params renders the query as Graph query parameters.
func (q ChannelQuery) Params() msgraph.Params {
	return msgraph.Params{
		Select: q.Select,
		Where:  q.Where,
		Custom: q.Custom,
	}
}

// ChannelMessageQuery selects, windows, and orders a channel message
// listing.
type ChannelMessageQuery struct {
	MessageQuery
	// WithReplies expands each root message's first page of replies. This is
	// the one $expand a channel message listing accepts; every other
	// $select or $expand is refused with 400, same as on a chat message
	// listing. A thread longer than the expansion's cap is truncated with a
	// per-message continuation link on the message itself -- see
	// [Client.MoreReplies].
	WithReplies bool
}

// Params renders the query as Graph query parameters.
func (q ChannelMessageQuery) Params() msgraph.Params {
	params := q.MessageQuery.Params()
	if q.WithReplies {
		params.Expand = append(params.Expand, "replies")
	}
	return params
}

// teamChannelsPath returns the channel collection path for a team.
func teamChannelsPath(team TeamID) (string, error) {
	if strings.TrimSpace(team.String()) == "" {
		return "", fmt.Errorf("%w: team id", ErrEmptyID)
	}
	return "/teams/" + escapeID(team.String()) + "/channels", nil
}

// teamChannelPath addresses one channel within a team.
func teamChannelPath(team TeamID, channel ChannelID) (string, error) {
	path, err := teamChannelsPath(team)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(channel.String()) == "" {
		return "", fmt.Errorf("%w: channel id", ErrEmptyID)
	}
	return path + "/" + escapeID(channel.String()), nil
}

// Channels iterates a team's channels.
//
// Needs [ListChannels].
func (c *Client) Channels(ctx context.Context, team TeamID, query ChannelQuery) iter.Seq2[Channel, error] {
	path, err := teamChannelsPath(team)
	if err != nil {
		return failed[Channel](err)
	}
	return items[Channel](ctx, c, path, query.Params())
}

// GetChannel fetches a single channel.
//
// Needs [ListChannels].
func (c *Client) GetChannel(ctx context.Context, team TeamID, channel ChannelID) (Channel, *msgraph.Response, error) {
	path, err := teamChannelPath(team, channel)
	if err != nil {
		return Channel{}, nil, err
	}
	return msgraph.Do[Channel](ctx, c.graph, c.request(http.MethodGet, path, msgraph.Params{}, nil))
}

// ChannelMessages iterates a channel's messages, following pagination.
//
// This returns ROOT messages only, at any page depth -- however far a
// caller pages, no reply ever appears in this collection, whether or not
// [ChannelMessageQuery.WithReplies] was set. A transcript built from this
// alone shows every conversation opener and none of the conversation. Read
// a thread with [Client.Replies] and [Client.MoreReplies], or expand the
// first page of each thread inline with WithReplies.
//
// Needs [ReadChannelMessages].
func (c *Client) ChannelMessages(ctx context.Context, team TeamID, channel ChannelID, query ChannelMessageQuery) iter.Seq2[ChatMessage, error] {
	path, err := teamChannelPath(team, channel)
	if err != nil {
		return failed[ChatMessage](err)
	}
	if err := query.Validate(); err != nil {
		return failed[ChatMessage](err)
	}
	return items[ChatMessage](ctx, c, path+"/messages", query.Params())
}

// Replies iterates the replies to a channel message, following pagination.
//
// The reply thread is one level deep: a reply's own id cannot be passed
// here to read further replies, because there are none -- see
// [Client.ReplyToChannelMessage].
//
// Needs [ReadChannelMessages].
func (c *Client) Replies(ctx context.Context, team TeamID, channel ChannelID, message MessageID, query MessageQuery) iter.Seq2[ChatMessage, error] {
	path, err := teamChannelPath(team, channel)
	if err != nil {
		return failed[ChatMessage](err)
	}
	if strings.TrimSpace(message.String()) == "" {
		return failed[ChatMessage](fmt.Errorf("%w: message id", ErrEmptyID))
	}
	if err := query.Validate(); err != nil {
		return failed[ChatMessage](err)
	}
	return items[ChatMessage](ctx, c, path+"/messages/"+escapeID(message.String())+"/replies", query.Params())
}

// MoreReplies continues a truncated, expanded replies list.
//
// [ChatMessage.RepliesNextLink] is a second pagination nested inside the
// message collection's own @odata.nextLink: the outer link pages root
// messages and says nothing about a reply thread that did not fit in the
// expansion. Nothing in [Client.ChannelMessages]'s own iteration follows
// it, so a caller reading only the outer link truncates every long thread
// silently, mid-argument, with no error and no gap it can detect.
//
// This follows the link directly with [Client.Graph], which is safe because
// msgraph-go pins requests to the configured Graph host regardless of what
// an absolute link names. A message with no RepliesNextLink yields nothing,
// not an error: there is nothing more to fetch.
//
// Needs [ReadChannelMessages].
func (c *Client) MoreReplies(ctx context.Context, message ChatMessage) iter.Seq2[ChatMessage, error] {
	if message.RepliesNextLink == "" {
		return func(func(ChatMessage, error) bool) {}
	}
	return items[ChatMessage](ctx, c, message.RepliesNextLink, msgraph.Params{})
}

// SendChannelMessage posts a new root message to a channel and returns the
// created copy.
//
// Needs [SendChannelMessage].
func (c *Client) SendChannelMessage(ctx context.Context, team TeamID, channel ChannelID, draft Draft) (ChatMessage, *msgraph.Response, error) {
	path, err := teamChannelPath(team, channel)
	if err != nil {
		return ChatMessage{}, nil, err
	}
	if draft.IsEmpty() {
		return ChatMessage{}, nil, ErrEmptyBody
	}
	return msgraph.Do[ChatMessage](ctx, c.graph,
		c.request(http.MethodPost, path+"/messages", msgraph.Params{}, draft))
}

// ReplyToChannelMessage posts a reply to a root channel message.
//
// The thread is one level deep. Posting to a reply's own id as though it
// were a root message fails, because a reply has no replies collection of
// its own -- see [Client.Replies].
//
// Needs [SendChannelMessage].
func (c *Client) ReplyToChannelMessage(ctx context.Context, team TeamID, channel ChannelID, message MessageID, draft Draft) (ChatMessage, *msgraph.Response, error) {
	path, err := teamChannelPath(team, channel)
	if err != nil {
		return ChatMessage{}, nil, err
	}
	if strings.TrimSpace(message.String()) == "" {
		return ChatMessage{}, nil, fmt.Errorf("%w: message id", ErrEmptyID)
	}
	if draft.IsEmpty() {
		return ChatMessage{}, nil, ErrEmptyBody
	}
	return msgraph.Do[ChatMessage](ctx, c.graph,
		c.request(http.MethodPost, path+"/messages/"+escapeID(message.String())+"/replies", msgraph.Params{}, draft))
}
