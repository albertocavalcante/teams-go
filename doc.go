// Package teams is a Microsoft Teams SDK over Microsoft Graph.
//
// It owns Teams semantics -- paths, request shapes, and the chat and channel
// message models -- and delegates transport, retries, paging, and error
// classification to msgraph-go. It does not do auth: build a
// [github.com/albertocavalcante/msgraph-go.Client] with a token source, then
// wrap it with [New].
//
// Three Graph behaviors shape the API. First, a $filter on a chat message
// listing is silently discarded unless $orderby names the same property --
// no error, the whole unfiltered collection comes back instead. Second,
// messageType is an evolvable enum: without a Prefer header asking Graph to
// include unknown enum members, every system event message -- a member
// joining, a chat renamed, a call ending -- arrives as unknownFutureValue,
// which in a real conversation's history is a large minority of it. [New]
// sets that header on every request, with no option to turn it off. Third,
// from is null on system event messages: the event has no author in the
// ordinary sense, and code that assumes otherwise renders a blank sender for
// a message that was never really "from" anyone.
package teams
