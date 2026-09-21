package teams

import (
	"errors"
	"strings"

	msgraph "github.com/albertocavalcante/msgraph-go"
)

// Errors returned when a call is malformed before it reaches Graph.
var (
	ErrNilGraphClient = errors.New("teams: nil graph client")
	ErrEmptyID        = errors.New("teams: empty identifier")
	ErrEmptyBody      = errors.New("teams: request body has no properties set")
)

// Graph error codes Teams returns for conditions worth handling.
const (
	// CodeBadRequest is what a chat or channel message listing answers for an
	// unsupported $orderby property, an ascending sort, or a $select/$expand
	// it does not serve. Unlike a mismatched $filter -- which the service
	// drops silently -- these shapes are genuinely refused.
	CodeBadRequest = "BadRequest"
	// CodeInvalidSearchRequest is what /search/query answers for a search
	// body missing entityTypes, or one that asks for sortProperties on a
	// chatMessage search, which the service will not sort.
	CodeInvalidSearchRequest = "invalidRequest"
)

// IsNotFound reports whether the chat, channel, team, or message does not
// exist.
func IsNotFound(err error) bool { return msgraph.IsNotFound(err) }

// IsForbidden reports whether the caller lacks the permission the request
// needed, or the tenant has otherwise refused it outright.
func IsForbidden(err error) bool { return msgraph.IsForbidden(err) }

// IsThrottled reports whether Graph rate-limited the request.
func IsThrottled(err error) bool { return msgraph.IsThrottled(err) }

// IsUnauthorized reports whether the request carried no usable token.
func IsUnauthorized(err error) bool { return msgraph.IsUnauthorized(err) }

// IsMessagingPolicyBlocked reports whether a send was refused by a tenant
// messaging policy rather than by a missing permission.
//
// Teams has no error code of its own for this: the response is a plain 403
// Forbidden, the same status a missing scope produces, so status alone
// cannot tell the two apart. The distinguishing text lives in the message,
// which is why this checks for it -- best-effort, because Microsoft has not
// documented the message as stable. The distinction still matters: a missing
// scope is fixed by asking the user to consent to more; a messaging policy
// is a tenant administrator's choice that the signed-in user cannot act on
// no matter what they consent to, so retrying after a new sign-in only loops.
func IsMessagingPolicyBlocked(err error) bool {
	if !msgraph.IsForbidden(err) {
		return false
	}
	var apiErr *msgraph.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return strings.Contains(strings.ToLower(apiErr.Message), "messaging policy")
}
