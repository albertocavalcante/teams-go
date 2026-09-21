package teams

import (
	"strings"
	"time"
)

// BodyType is the content type of an [ItemBody].
type BodyType string

const (
	BodyTypeText BodyType = "text"
	BodyTypeHTML BodyType = "html"
)

// ItemBody is a message or draft body.
type ItemBody struct {
	ContentType BodyType `json:"contentType,omitempty"`
	Content     string   `json:"content,omitempty"`
}

// TextBody builds a plain-text body.
func TextBody(content string) *ItemBody {
	return &ItemBody{ContentType: BodyTypeText, Content: content}
}

// HTMLBody builds an HTML body.
func HTMLBody(content string) *ItemBody {
	return &ItemBody{ContentType: BodyTypeHTML, Content: content}
}

// IdentityType is Graph's teamworkUserIdentityType enum, present on identities
// that come from presence and meeting-participant endpoints. A chat message's
// sender does not carry one; the zero value there means "not applicable", not
// "aadUser".
type IdentityType string

const (
	IdentityTypeAADUser                  IdentityType = "aadUser"
	IdentityTypeOnPremiseAADUser         IdentityType = "onPremiseAadUser"
	IdentityTypeAnonymousGuest           IdentityType = "anonymousGuest"
	IdentityTypeFederatedUser            IdentityType = "federatedUser"
	IdentityTypePersonalMicrosoftAccount IdentityType = "personalMicrosoftAccount"
	IdentityTypeSkypeUser                IdentityType = "skypeUser"
	IdentityTypePhoneUser                IdentityType = "phoneUser"
	IdentityTypeUnknownFutureValue       IdentityType = "unknownFutureValue"
)

// Identity is a single Graph identity resource: a person, application, or
// device with an id and a display name.
type Identity struct {
	ID          string       `json:"id,omitempty"`
	DisplayName string       `json:"displayName,omitempty"`
	Type        IdentityType `json:"userIdentityType,omitempty"`
}

// IdentityKind names which slot of an [IdentitySet] was populated.
type IdentityKind string

const (
	IdentityKindUser        IdentityKind = "user"
	IdentityKindApplication IdentityKind = "application"
	IdentityKindDevice      IdentityKind = "device"
)

// IdentitySet is Graph's identitySet resource: at most one of User,
// Application, or Device is populated, naming whatever sent the message. On a
// system event message every field is nil, because nothing sent it.
type IdentitySet struct {
	User        *Identity `json:"user,omitempty"`
	Application *Identity `json:"application,omitempty"`
	Device      *Identity `json:"device,omitempty"`
}

// Actor returns whichever identity is populated and reports which kind it
// was. It is nil-receiver-safe: a caller holding a *IdentitySet obtained from
// [ChatMessage.From] does not have to check for nil before calling this, only
// before doing anything with the returned bool.
func (s *IdentitySet) Actor() (Identity, IdentityKind, bool) {
	if s == nil {
		return Identity{}, "", false
	}
	switch {
	case s.User != nil:
		return *s.User, IdentityKindUser, true
	case s.Application != nil:
		return *s.Application, IdentityKindApplication, true
	case s.Device != nil:
		return *s.Device, IdentityKindDevice, true
	default:
		return Identity{}, "", false
	}
}

// Name returns the populated identity's display name, or the empty string
// when the set is nil or empty. It is nil-receiver-safe for the same reason
// [IdentitySet.Actor] is.
func (s *IdentitySet) Name() string {
	identity, _, ok := s.Actor()
	if !ok {
		return ""
	}
	return identity.DisplayName
}

// MentionedIdentitySet is Graph's chatMessageMentionedIdentitySet: wider than
// [IdentitySet] because a mention can also name the conversation itself or a
// tag, neither of which is ever the sender of a message.
type MentionedIdentitySet struct {
	User         *Identity `json:"user,omitempty"`
	Application  *Identity `json:"application,omitempty"`
	Device       *Identity `json:"device,omitempty"`
	Conversation *Identity `json:"conversation,omitempty"`
	Tag          *Identity `json:"tag,omitempty"`
}

// name returns whichever identity is populated, or the empty string. Unlike
// [IdentitySet.Actor] this does not report which slot matched: the caller
// resolving a mention only ever wants a display name to show.
func (s *MentionedIdentitySet) name() string {
	if s == nil {
		return ""
	}
	for _, identity := range []*Identity{s.User, s.Application, s.Device, s.Conversation, s.Tag} {
		if identity != nil && identity.DisplayName != "" {
			return identity.DisplayName
		}
	}
	return ""
}

// Mention is one entry in a chat message's mentions list, matched against an
// <at id="N"> tag in the body by [Mention.ID].
type Mention struct {
	ID          int32                 `json:"id"`
	MentionText string                `json:"mentionText,omitempty"`
	Mentioned   *MentionedIdentitySet `json:"mentioned,omitempty"`
}

// ReactionType names a message reaction. It is a plain string rather than a
// closed enum: a tenant can enable custom emoji reactions beyond the six
// built in ones, and a reaction using one arrives as whatever string the
// tenant named it. Treating this as an exhaustive switch would silently drop
// every custom reaction a message received.
type ReactionType string

const (
	ReactionLike      ReactionType = "like"
	ReactionHeart     ReactionType = "heart"
	ReactionLaugh     ReactionType = "laugh"
	ReactionSurprised ReactionType = "surprised"
	ReactionSad       ReactionType = "sad"
	ReactionAngry     ReactionType = "angry"
)

// Reaction is one reaction on a chat message.
type Reaction struct {
	ReactionType    ReactionType `json:"reactionType,omitempty"`
	CreatedDateTime time.Time    `json:"createdDateTime,omitzero"`
	// User is Graph's chatMessageReactionIdentitySet, which happens to share
	// IdentitySet's shape: the wire nests the reacting person one level
	// deeper than it looks, under this object's own "user" key, rather than
	// holding the identity directly. IdentitySet already models a "user" key
	// holding an Identity, so reusing it here decodes the extra nesting for
	// free -- the reacting person is User.User, reached the same way as any
	// other IdentitySet, via [IdentitySet.Actor] or [IdentitySet.Name].
	User *IdentitySet `json:"user,omitempty"`
}

// MessageAttachment is a chat or channel message's attachment metadata.
type MessageAttachment struct {
	ID           string `json:"id,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	ContentURL   string `json:"contentUrl,omitempty"`
	Content      string `json:"content,omitempty"`
	Name         string `json:"name,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

// Importance is a chat message's importance level.
type Importance string

const (
	ImportanceNormal Importance = "normal"
	ImportanceHigh   Importance = "high"
	ImportanceUrgent Importance = "urgent"
)

// ChannelIdentity names the team and channel a channel message belongs to.
type ChannelIdentity struct {
	TeamID    TeamID    `json:"teamId,omitempty"`
	ChannelID ChannelID `json:"channelId,omitempty"`
}

// MessageType is Graph's evolvable chatMessageType enum. See doc.go for what
// it costs a caller who does not send [msgraph.PreferIncludeUnknownEnumMembers]:
// every system event message decodes to [MessageTypeUnknownFutureValue]
// instead of [MessageTypeSystemEventMessage].
type MessageType string

const (
	MessageTypeMessage            MessageType = "message"
	MessageTypeChatEvent          MessageType = "chatEvent"
	MessageTypeTyping             MessageType = "typing"
	MessageTypeSystemEventMessage MessageType = "systemEventMessage"
	MessageTypeUnknownFutureValue MessageType = "unknownFutureValue"
)

// ChatMessage is a message in a chat or a channel.
//
// Graph omits any property that was not requested or expanded, so a field
// being zero can mean either "empty" or "not populated for this request".
type ChatMessage struct {
	ID MessageID `json:"id,omitempty"`
	// ETag is the message's version tag, exposed on the wire as @odata.etag.
	ETag                 ETag        `json:"@odata.etag,omitempty"`
	MessageType          MessageType `json:"messageType,omitempty"`
	CreatedDateTime      time.Time   `json:"createdDateTime,omitzero"`
	LastModifiedDateTime time.Time   `json:"lastModifiedDateTime,omitzero"`
	// LastEditedDateTime is set only when a human edited the message after
	// posting it, which is what makes it the right field for
	// [ChatMessage.Edited]. LastModifiedDateTime is the wrong one to use for
	// that: it also moves when someone reacts to the message, so a message
	// nobody has touched would misreport as edited the first time it gets a
	// thumbs up.
	LastEditedDateTime *time.Time `json:"lastEditedDateTime,omitempty"`
	// DeletedDateTime is set when the message was soft-deleted. The message
	// still exists at this ID with its body cleared by the service, rather
	// than the resource disappearing.
	DeletedDateTime *time.Time `json:"deletedDateTime,omitempty"`

	// From is null on a system event message: nothing sent it, the service
	// generated it. Dereferencing this unconditionally is the most common way
	// a Teams client renders a blank sender for a membership change instead
	// of recognizing the message as an event. See [ChatMessage.Sender] and
	// [ChatMessage.IsSystemEvent].
	From *IdentitySet `json:"from,omitempty"`
	Body *ItemBody    `json:"body,omitempty"`

	Subject string `json:"subject,omitempty"`
	Summary string `json:"summary,omitempty"`

	Mentions    []Mention           `json:"mentions,omitempty"`
	Reactions   []Reaction          `json:"reactions,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`

	Importance Importance `json:"importance,omitempty"`
	Locale     string     `json:"locale,omitempty"`
	WebURL     string     `json:"webUrl,omitempty"`

	// EventDetail is set only on a system event message, whichever
	// MessageType it decoded to. [ChatMessage.IsSystemEvent] checks this
	// rather than MessageType, which is what keeps it correct even when a
	// proxy strips the Prefer header this package always sends and the type
	// arrives as [MessageTypeUnknownFutureValue] instead.
	EventDetail *EventDetail `json:"eventDetail,omitempty"`

	ChatID          ChatID          `json:"chatId,omitempty"`
	ChannelIdentity ChannelIdentity `json:"channelIdentity,omitempty"`
	ReplyToID       MessageID       `json:"replyToId,omitempty"`

	// Replies is populated only when the request expanded them. An empty
	// slice here does not mean the message has no replies; it means they were
	// not asked for.
	Replies []ChatMessage `json:"replies,omitempty"`
	// RepliesNextLink continues a truncated, expanded replies list. It is a
	// second pagination nested inside the parent collection's own nextLink; a
	// caller following only the outer link truncates every long thread
	// silently, with no error and no gap it can detect.
	RepliesNextLink string `json:"replies@odata.nextLink,omitempty"`
}

// Sender returns the message's author, reporting false for a system event
// message, which has none.
func (m ChatMessage) Sender() (Identity, bool) {
	identity, _, ok := m.From.Actor()
	return identity, ok
}

// Mine reports whether self authored the message. It is false for a system
// event message and false when self is empty, rather than matching against a
// sender the caller did not actually ask about.
func (m ChatMessage) Mine(self UserID) bool {
	if self == "" {
		return false
	}
	sender, ok := m.Sender()
	return ok && sender.ID == string(self)
}

// IsSystemEvent reports whether the message is service-generated rather than
// authored. It checks EventDetail rather than MessageType, so it stays
// correct even when a proxy strips the Prefer header this package always
// sends and the type decodes to [MessageTypeUnknownFutureValue] instead of
// [MessageTypeSystemEventMessage].
func (m ChatMessage) IsSystemEvent() bool { return m.EventDetail != nil }

// Deleted reports whether the message was soft-deleted. It still exists at
// this ID; only its content is gone.
func (m ChatMessage) Deleted() bool { return m.DeletedDateTime != nil }

// Edited reports whether a human edited the message after posting it. See the
// field comment on [ChatMessage.LastEditedDateTime] for why this is not
// LastModifiedDateTime.
func (m ChatMessage) Edited() bool { return m.LastEditedDateTime != nil }

// Draft is the set of properties Graph accepts when creating a chat or
// channel message.
//
// It is a separate type from [ChatMessage] because the send endpoint accepts
// only a handful of properties and silently discards the rest: posting a
// round-tripped ChatMessage sends an id, an etag, and timestamps that Graph
// ignores, which reads as "it worked" right up until a caller relies on one
// of those fields meaning something in the response.
type Draft struct {
	Body        *ItemBody           `json:"body"`
	Subject     string              `json:"subject,omitempty"`
	Importance  Importance          `json:"importance,omitempty"`
	Mentions    []Mention           `json:"mentions,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
}

// IsEmpty reports whether sending the draft would post nothing: no body text,
// no subject, no importance, no mentions, and no attachments.
func (d Draft) IsEmpty() bool {
	hasBody := d.Body != nil && strings.TrimSpace(d.Body.Content) != ""
	return !hasBody && d.Subject == "" && d.Importance == "" &&
		len(d.Mentions) == 0 && len(d.Attachments) == 0
}
