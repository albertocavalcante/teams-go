package teams

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"strconv"
	"time"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// Identifiers are distinct types rather than bare strings. Chats, channels,
// teams, users, and hosted content are all opaque strings of the same shape
// on the wire, and passing one where another was expected would otherwise
// produce a confusing 404 from the service instead of a compile error.
type (
	// ChatID identifies a chat.
	ChatID string
	// TeamID identifies a team.
	TeamID string
	// ChannelID identifies a channel within a team.
	ChannelID string
	// UserID identifies a user, typically an Azure AD object ID.
	UserID string
	// HostedContentID identifies an inline image or file referenced from a
	// message body.
	HostedContentID string

	// MessageID identifies a chat or channel message.
	//
	// It is epoch milliseconds, assigned by Graph when the message was
	// created, and it is unique only within the chat, channel, or reply
	// thread it was posted to -- the same numeric ID can name two entirely
	// different messages in two different chats. That is why nothing in this
	// package accepts a MessageID on its own: every operation also names the
	// chat, channel, or thread the message belongs to.
	MessageID string

	// ETag is a resource version tag, used to make a write conditional.
	ETag string
)

func (i ChatID) String() string          { return string(i) }
func (i TeamID) String() string          { return string(i) }
func (i ChannelID) String() string       { return string(i) }
func (i UserID) String() string          { return string(i) }
func (i HostedContentID) String() string { return string(i) }
func (i MessageID) String() string       { return string(i) }
func (e ETag) String() string            { return string(e) }

// Time parses a MessageID back into the instant Graph assigned it, reporting
// false rather than the zero time when the ID is not a decimal number. ID "0"
// and an ID that is not numeric at all are different facts, and collapsing
// both into time.Time{} would make a malformed ID indistinguishable from a
// message posted at the Unix epoch.
func (i MessageID) Time() (time.Time, bool) {
	millis, err := strconv.ParseInt(string(i), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(millis), true
}

// Client is a Microsoft Teams client over Microsoft Graph.
//
// It owns Teams semantics -- paths, request shapes, and models -- and
// delegates transport, retries, paging, and error classification to
// [msgraph.Client].
type Client struct {
	graph *msgraph.Client
	self  UserID
	// prefer is sent with every request. New always appends
	// [msgraph.PreferIncludeUnknownEnumMembers] to it; see [New].
	prefer []msgraph.PreferDirective
	// capNotify, when set, is told about a request an endpoint's own cap
	// capped below what was asked for. Endpoints outside this foundation
	// package -- chats, channel messages, search -- own the specific cap
	// values and call [Client.notifyCap] after clamping with [clampTop].
	capNotify func(requested, served int)
	// maxPages bounds a single traversal. Zero uses
	// [msgraph.DefaultMaxPages].
	maxPages int
}

// Option configures a [Client].
type Option func(*Client) error

// WithSelf records the signed-in user's ID, so [ChatMessage.Mine] and future
// call sites that need to distinguish "I sent this" from "someone else sent
// this" do not each have to be told separately.
func WithSelf(user UserID) Option {
	return func(c *Client) error {
		c.self = user
		return nil
	}
}

// WithMaxPages bounds how many pages a single listing will follow, replacing
// [msgraph.DefaultMaxPages].
//
// The bound exists because refusing to repeat a link is not enough. A service
// that mints a fresh nextLink every time is followed forever, and the set of
// links already seen grows by one URL per page while it happens, so the run
// consumes requests and memory without end. A page count is the only thing
// that stops it.
func WithMaxPages(pages int) Option {
	return func(c *Client) error {
		if pages <= 0 {
			return fmt.Errorf("teams: max pages must be positive, got %d", pages)
		}
		c.maxPages = pages
		return nil
	}
}

// WithPageCapNotify registers a callback told when a request was served
// fewer items than it asked for because an endpoint capped it -- Teams
// listings do this silently, with no error and no indication in the response
// that anything was capped. notify is called with the number requested and
// the number actually served.
func WithPageCapNotify(notify func(requested, served int)) Option {
	return func(c *Client) error {
		c.capNotify = notify
		return nil
	}
}

// WithPrefer adds arbitrary Prefer directives to every request.
func WithPrefer(directives ...msgraph.PreferDirective) Option {
	return func(c *Client) error {
		c.prefer = append(c.prefer, directives...)
		return nil
	}
}

// New creates a Teams client over an existing Graph client.
//
// It always appends [msgraph.PreferIncludeUnknownEnumMembers] to the Prefer
// directives sent with every request, and there is no option to turn this
// off. Without it, every system event message -- a member joining, a chat
// renamed, a call ending -- decodes to [MessageTypeUnknownFutureValue]
// instead of [MessageTypeSystemEventMessage], silently and with no error; in
// a real chat's history that is a large minority of it. There is no
// legitimate reason to want that, so it is not a choice this package offers.
func New(graph *msgraph.Client, opts ...Option) (*Client, error) {
	if graph == nil {
		return nil, ErrNilGraphClient
	}
	c := &Client{graph: graph}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	c.prefer = append(c.prefer, msgraph.PreferIncludeUnknownEnumMembers)
	return c, nil
}

// Graph returns the underlying Graph client for calls this package does not
// model.
func (c *Client) Graph() *msgraph.Client { return c.graph }

// Self returns the signed-in user's ID configured with [WithSelf], or the
// zero [UserID] if none was set.
func (c *Client) Self() UserID { return c.self }

// notifyCap tells the configured [WithPageCapNotify] callback, if any, that a
// request asking for requested items was served only served of them. It is a
// no-op with no callback configured, so a call site does not have to
// nil-check before calling it.
func (c *Client) notifyCap(requested, served int) {
	if c.capNotify != nil {
		c.capNotify(requested, served)
	}
}

// clampTop returns max when requested exceeds it, and returns requested
// otherwise -- including zero, which asks Graph for its own default page
// size rather than "no limit". Substituting max for a zero would ask for more
// than the caller specified.
func clampTop(requested, limit int) int {
	if requested > limit {
		return limit
	}
	return requested
}

// request builds a Graph request carrying the client's Prefer directives.
func (c *Client) request(method, path string, params msgraph.Params, body any, extra ...msgraph.PreferDirective) msgraph.Request {
	prefer := c.prefer
	if len(extra) > 0 {
		prefer = append(append([]msgraph.PreferDirective(nil), c.prefer...), extra...)
	}
	return msgraph.Request{
		Method: method,
		URL:    path,
		Params: params,
		Prefer: prefer,
		Body:   body,
	}
}

// paginate walks a Graph collection, carrying the client's Prefer directives
// onto every page.
//
// It carries two independent bounds because they catch different failures. A
// service that repeats a link is caught by the seen map; one that mints a
// fresh link every time is not, and would be followed forever -- which also
// grows the seen map by one URL per page the whole time it happens. Only a
// page count stops that one.
func paginate[T any](
	ctx context.Context,
	c *Client,
	path string,
	params msgraph.Params,
	extra ...msgraph.PreferDirective,
) iter.Seq2[msgraph.Page[T], error] {
	return func(yield func(msgraph.Page[T], error) bool) {
		next := path
		seen := map[string]bool{}
		maxPages := c.maxPages
		if maxPages <= 0 {
			maxPages = msgraph.DefaultMaxPages
		}
		pages := 0

		for next != "" {
			if seen[next] {
				_ = yield(msgraph.Page[T]{}, fmt.Errorf("%w: %s", msgraph.ErrPageCycle, next))
				return
			}
			if pages >= maxPages {
				_ = yield(msgraph.Page[T]{}, fmt.Errorf("%w: stopped after %d pages",
					msgraph.ErrMaxPagesExceeded, pages))
				return
			}
			seen[next] = true
			pages++

			page, _, err := msgraph.Do[msgraph.Page[T]](ctx, c.graph,
				c.request(http.MethodGet, next, params, nil, extra...))
			if err != nil {
				_ = yield(msgraph.Page[T]{}, err)
				return
			}
			if !yield(page, nil) {
				return
			}
			// A nextLink already encodes the query that produced it, so
			// re-applying params on the next request would duplicate or
			// contradict it.
			next = page.NextLink
			params = msgraph.Params{}
		}
	}
}

// items flattens paginate into a per-item iterator.
func items[T any](
	ctx context.Context,
	c *Client,
	path string,
	params msgraph.Params,
	extra ...msgraph.PreferDirective,
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for page, err := range paginate[T](ctx, c, path, params, extra...) {
			if err != nil {
				var zero T
				_ = yield(zero, err)
				return
			}
			for i := range page.Value {
				if !yield(page.Value[i], nil) {
					return
				}
			}
		}
	}
}

// failed returns an iterator that yields a single error, for the case where a
// request could not be built at all -- an empty ID, say -- and there is
// nothing to page.
func failed[T any](err error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		_ = yield(zero, err)
	}
}
