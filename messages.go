package teams

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"time"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// MessageOrder names the property a message listing is sorted and windowed
// by. See [MessageQuery] for why this is one field rather than a separate
// filter property and sort property.
type MessageOrder string

const (
	// ByCreated orders by createdDateTime. It is MessageQuery's zero value.
	ByCreated MessageOrder = "createdDateTime"
	// ByLastModified orders by lastModifiedDateTime, which also moves when a
	// message is reacted to, edited, or soft-deleted -- so a listing ordered
	// by it surfaces recently-touched messages, not only recently-sent ones.
	ByLastModified MessageOrder = "lastModifiedDateTime"
)

// property returns the OData property name for o, treating any value other
// than [ByLastModified] -- including the zero value -- as [ByCreated].
func (o MessageOrder) property() string {
	if o == ByLastModified {
		return string(ByLastModified)
	}
	return string(ByCreated)
}

const (
	// MaxMessagePageSize is the largest page a chat or channel message
	// listing serves, whatever $top asks for. A request above it is not
	// refused; it is silently served this many.
	MaxMessagePageSize = 50
	// DefaultChannelMessagePageSize is what a channel message listing
	// returns when $top says nothing. It is well under
	// [MaxMessagePageSize], so leaving Top unset is not the same as asking
	// for the largest page the service will give.
	DefaultChannelMessagePageSize = 20
)

// ErrInvertedWindow means a [MessageQuery]'s Since is after its Before.
//
// The service does not refuse this shape: it answers 200 OK with an empty
// page, which reads exactly like a quiet chat rather than like a mistake.
// [MessageQuery.Validate] exists so the mistake is caught here, with a
// message that says which end was wrong, instead of silently as "nothing
// happened here."
var ErrInvertedWindow = errors.New("teams: message window is inverted: Since is after Before")

// MessageQuery selects and windows a chat message listing.
//
// # Why Order is one field, not a filter property and a sort property
//
// A chat message listing is indexed on exactly one property at a time, and
// the service does not tell you when a request asks for two: a $filter on
// createdDateTime combined with $orderby on lastModifiedDateTime is not
// refused. It is answered 200 OK, with the filter silently discarded and
// the whole collection returned instead. A caller asking for the last day
// of a chat gets a year of it back, and a UI that trusts the response
// renders a year of messages as though they were all from today, with
// nothing in the response shape to suggest anything went wrong.
//
// Order also selects the property used by Before and Since. Params renders
// Before as a server filter on that property; Messages and ListMessages
// enforce Since locally because Graph does not accept an inclusive lower
// bound for either supported timestamp. See [FieldLastMessagePreviewCreated]
// and [ChatQuery] for a similar filter/order constraint on chat listings.
//
// # Why there is no Select, Expand, Where, or Descending field
//
// A chat message listing refuses $select and $expand outright
// with 400 Bad Request -- unlike the filter mismatch above, this one really
// is an error, which is itself worth knowing, since the two look identical
// from the caller's side until one of them silently succeeds. The service
// only serves descending order; an ascending $orderby is refused. Fields
// that a request would only be refused for existing are not offered here at
// all, so the type cannot express the requests that never work.
type MessageQuery struct {
	// Order is the property the listing is filtered and sorted on. The zero
	// value is [ByCreated].
	Order MessageOrder
	// Since is an inclusive lower bound on Order. Graph cannot express this
	// bound for createdDateTime, so the client stops paging once it reaches
	// an older message. The zero time omits it.
	Since time.Time
	// Before is an exclusive upper bound on Order. The zero time omits it.
	Before time.Time
	// Top asks for a page size, clamped to [MaxMessagePageSize]. Zero asks
	// for the service's own default rather than the maximum.
	Top int
	// Custom carries query parameters this package does not model.
	Custom url.Values
}

// Params renders the server-supported part of the query. Graph supports an
// exclusive upper bound on either timestamp, but no inclusive lower bound.
// Messages and ListMessages apply Since after fetching each page.
func (q MessageQuery) Params() msgraph.Params {
	field := q.Order.property()
	var before msgraph.Expr
	if !q.Before.IsZero() {
		before = msgraph.Lt(field, q.Before)
	}
	return msgraph.Params{
		Where:  before,
		Sort:   []msgraph.Order{msgraph.Desc(field)},
		Top:    clampTop(q.Top, MaxMessagePageSize),
		Custom: q.Custom,
	}
}

// ErrMissingMessageTimestamp means a response omitted the property needed to
// enforce Since. Returning an error avoids silently presenting older messages
// as though they matched the requested window.
var ErrMissingMessageTimestamp = errors.New("teams: message has no timestamp for the requested order")

// olderThanSince reports whether a descending listing has reached the end of
// the requested window. The matching property must be populated by Graph.
func (q MessageQuery) olderThanSince(message *ChatMessage) (bool, error) {
	if q.Since.IsZero() {
		return false, nil
	}
	timestamp := message.CreatedDateTime
	if q.Order == ByLastModified {
		timestamp = message.LastModifiedDateTime
	}
	if timestamp.IsZero() {
		return false, fmt.Errorf("%w: %s on message %s",
			ErrMissingMessageTimestamp, q.Order.property(), message.ID)
	}
	return timestamp.Before(q.Since), nil
}

// Validate reports [ErrInvertedWindow] when Since is after Before. Call it
// before sending the query; every operation in this file does, so this is
// mainly useful to a caller building a [MessageQuery] to hand to
// [Client.ListMessages] or [Client.Messages] and wanting to fail before the
// round trip rather than after it.
func (q MessageQuery) Validate() error {
	if q.Since.IsZero() || q.Before.IsZero() {
		return nil
	}
	if q.Since.After(q.Before) {
		return fmt.Errorf("%w: %s is after %s",
			ErrInvertedWindow, q.Since.Format(time.RFC3339), q.Before.Format(time.RFC3339))
	}
	return nil
}

// escapeID percent-encodes an identifier for use as a single path segment.
// Teams identifiers routinely contain characters -- ':', '@', '/' inside a
// thread id -- that change a URL's meaning if passed through unescaped.
func escapeID(id string) string { return url.PathEscape(id) }

// chatMessagesPath returns the message collection path for a chat.
func chatMessagesPath(chat ChatID) (string, error) {
	path, err := chatPath(chat)
	if err != nil {
		return "", err
	}
	return path + "/messages", nil
}

// chatMessagePath addresses one message within a chat, optionally with an
// action suffix such as "/setReaction".
func chatMessagePath(chat ChatID, message MessageID, suffix ...string) (string, error) {
	path, err := chatMessagesPath(chat)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message.String()) == "" {
		return "", fmt.Errorf("%w: message id", ErrEmptyID)
	}
	return path + "/" + escapeID(message.String()) + strings.Join(suffix, ""), nil
}

// Messages iterates a chat's messages, following pagination.
//
// Needs [ReadChatMessages].
func (c *Client) Messages(ctx context.Context, chat ChatID, query MessageQuery) iter.Seq2[ChatMessage, error] {
	path, err := chatMessagesPath(chat)
	if err != nil {
		return failed[ChatMessage](err)
	}
	if err := query.Validate(); err != nil {
		return failed[ChatMessage](err)
	}
	return func(yield func(ChatMessage, error) bool) {
		for message, itemErr := range items[ChatMessage](ctx, c, path, query.Params()) {
			if itemErr != nil {
				yield(ChatMessage{}, itemErr)
				return
			}
			older, err := query.olderThanSince(&message)
			if err != nil {
				yield(ChatMessage{}, err)
				return
			}
			if older {
				return
			}
			if !yield(message, nil) {
				return
			}
		}
	}
}

// ListMessages returns one page of a chat's messages plus the
// [msgraph.Response] metadata. When Since is set, it removes older items and
// clears NextLink once the descending page crosses that bound. Use
// [Client.Messages] to page automatically.
//
// Needs [ReadChatMessages].
func (c *Client) ListMessages(ctx context.Context, chat ChatID, query MessageQuery) (msgraph.Page[ChatMessage], *msgraph.Response, error) {
	path, err := chatMessagesPath(chat)
	if err != nil {
		return msgraph.Page[ChatMessage]{}, nil, err
	}
	if err := query.Validate(); err != nil {
		return msgraph.Page[ChatMessage]{}, nil, err
	}
	page, response, err := msgraph.Do[msgraph.Page[ChatMessage]](ctx, c.graph,
		c.request(http.MethodGet, path, query.Params(), nil))
	if err != nil || query.Since.IsZero() {
		return page, response, err
	}
	filtered := page.Value[:0]
	for i := range page.Value {
		older, filterErr := query.olderThanSince(&page.Value[i])
		if filterErr != nil {
			return msgraph.Page[ChatMessage]{}, response, filterErr
		}
		if older {
			page.NextLink = ""
			break
		}
		filtered = append(filtered, page.Value[i])
	}
	page.Value = filtered
	return page, response, nil
}

// GetMessage fetches a single chat message.
//
// There is no query parameter, unlike [Client.GetChat] or [Client.Messages]:
// $select and $expand are unsupported on a message resource just as they are
// on the listing, so a query argument here would only let a caller build a
// request the service answers with 400.
//
// Needs [ReadChatMessages].
func (c *Client) GetMessage(ctx context.Context, chat ChatID, message MessageID) (ChatMessage, *msgraph.Response, error) {
	path, err := chatMessagePath(chat, message)
	if err != nil {
		return ChatMessage{}, nil, err
	}
	return msgraph.Do[ChatMessage](ctx, c.graph, c.request(http.MethodGet, path, msgraph.Params{}, nil))
}

// SendMessage posts a new message to a chat and returns the created copy.
//
// Needs [SendChatMessage].
func (c *Client) SendMessage(ctx context.Context, chat ChatID, draft Draft) (ChatMessage, *msgraph.Response, error) {
	path, err := chatMessagesPath(chat)
	if err != nil {
		return ChatMessage{}, nil, err
	}
	if draft.IsEmpty() {
		return ChatMessage{}, nil, ErrEmptyBody
	}
	return msgraph.Do[ChatMessage](ctx, c.graph, c.request(http.MethodPost, path, msgraph.Params{}, draft))
}

// SetReaction reacts to a message.
//
// Reacting again with the same type is idempotent. Reacting with a
// different type does not replace the first: the message ends up carrying
// both reactions from the same person, which surprises anyone expecting a
// radio button where the service implements a set. Call [Client.UnsetReaction]
// with the old type first to swap one reaction for another.
//
// Needs [ChangeChat].
func (c *Client) SetReaction(ctx context.Context, chat ChatID, message MessageID, reaction ReactionType) (*msgraph.Response, error) {
	path, err := chatMessagePath(chat, message, "/setReaction")
	if err != nil {
		return nil, err
	}
	body := map[string]string{"reactionType": string(reaction)}
	return c.graph.Do(ctx, c.request(http.MethodPost, path, msgraph.Params{}, body), nil)
}

// UnsetReaction removes a reaction of the given type. Removing a reaction
// that was never set is not an error.
//
// Needs [ChangeChat].
func (c *Client) UnsetReaction(ctx context.Context, chat ChatID, message MessageID, reaction ReactionType) (*msgraph.Response, error) {
	path, err := chatMessagePath(chat, message, "/unsetReaction")
	if err != nil {
		return nil, err
	}
	body := map[string]string{"reactionType": string(reaction)}
	return c.graph.Do(ctx, c.request(http.MethodPost, path, msgraph.Params{}, body), nil)
}

// SoftDeleteMessage deletes a message.
//
// The message is not removed. It keeps its id, keeps appearing in every
// listing that already reached it, and reports [ChatMessage.Deleted] true
// with its body emptied by the service. A caller that wants a soft-deleted
// message gone from a rendered transcript has to filter it out itself; the
// listing will not do that for them.
//
// Needs [ChangeChat].
func (c *Client) SoftDeleteMessage(ctx context.Context, chat ChatID, message MessageID) (*msgraph.Response, error) {
	path, err := chatMessagePath(chat, message, "/softDelete")
	if err != nil {
		return nil, err
	}
	return c.graph.Do(ctx, c.request(http.MethodPost, path, msgraph.Params{}, nil), nil)
}

// UndoSoftDeleteMessage restores a message soft-deleted with
// [Client.SoftDeleteMessage]. The body the service emptied on delete is not
// restored; only the deletedDateTime marker is cleared.
//
// Needs [ChangeChat].
func (c *Client) UndoSoftDeleteMessage(ctx context.Context, chat ChatID, message MessageID) (*msgraph.Response, error) {
	path, err := chatMessagePath(chat, message, "/undoSoftDelete")
	if err != nil {
		return nil, err
	}
	return c.graph.Do(ctx, c.request(http.MethodPost, path, msgraph.Params{}, nil), nil)
}
