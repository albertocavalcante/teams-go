package teams

// Message property paths, for naming a sort property without misspelling it
// into a request the service quietly answers wrong instead of rejecting; see
// [MessageQuery] in messages.go.
const (
	FieldCreatedDateTime      = "createdDateTime"
	FieldLastModifiedDateTime = "lastModifiedDateTime"
	// FieldLastMessagePreviewCreated is the property a chat listing orders by
	// when [ChatQuery.ByRecentMessage] is set. It lives one level into the
	// expanded lastMessagePreview, which is why [ChatQuery.Params] adds the
	// expansion automatically rather than leaving a caller to discover that
	// ordering by it silently does nothing without the expand.
	FieldLastMessagePreviewCreated = "lastMessagePreview/createdDateTime"
)

// There are deliberately no filter combinators here -- no And, Or, Contains,
// or property-specific predicate the way outlook-go's predicates.go builds
// them for mail.
//
// A mail filter can combine arbitrary properties because Exchange indexes
// several of them. A Teams message listing indexes exactly one thing: a
// window on whichever timestamp it is already sorted by. [MessageQuery]
// spells that window directly, as Since and Before, and always renders its
// $filter and $orderby against the same property -- see messages.go for why
// that coupling is load-bearing. A predicate builder here would only let a
// caller construct a filter on some other property, such as importance or a
// sender, which the service does not refuse: it silently discards the
// filter and returns the whole collection, successfully. Offering the
// combinator would be offering a way to build exactly the mistake this
// package exists to prevent.

// Newest orders a message listing by creation time, newest first. This is
// also [MessageQuery]'s zero value for Order, so passing it is never
// required -- it exists for a call site that wants to say the ordering out
// loud rather than lean on an implicit default.
func Newest() MessageOrder { return ByCreated }
