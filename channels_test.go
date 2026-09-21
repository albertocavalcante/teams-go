package teams_test

import (
	"context"
	"testing"

	"github.com/albertocavalcante/msgraph-go/graphmock"
	teams "github.com/albertocavalcante/teams-go"
)

// TestChannelListingReturnsRootsOnly pins that a channel message listing
// never surfaces a reply, however the fake mixes one in -- a client that
// treats the listing as a full transcript would otherwise render half of a
// conversation with no indication anything was missing.
func TestChannelListingReturnsRootsOnly(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	server.Handle("GET /teams/{team}/channels/{channel}/messages", func(*graphmock.Request) graphmock.Response {
		return graphmock.Collection([]teams.ChatMessage{
			{ID: "1", MessageType: teams.MessageTypeMessage},
			// A reply the fake mixes into the collection on purpose; the
			// service never does, and the shaper must drop it.
			{ID: "2", MessageType: teams.MessageTypeMessage, ReplyToID: "1"},
		}, "")
	})
	client := newTeamsClient(t, server)

	var got []teams.MessageID
	for msg, err := range client.ChannelMessages(context.Background(), "t1", "19:c@thread.tacv2", teams.ChannelMessageQuery{}) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, msg.ID)
	}
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("ChannelMessages returned %v, want only the root message", got)
	}
}

// TestExpandedRepliesTruncateAndAreResumable is the end-to-end version of the
// nested-paging claim: the fake truncates a long thread exactly as the service
// does, and the link it emits is one the client can actually follow.
//
// The hand-built variant below proves MoreReplies dereferences a link. This
// one proves the link a truncated thread carries is dereferenceable at all --
// which is the part that was briefly untestable, because the shaper first
// emitted an absolute graph.microsoft.com URL that the client's host pinning
// correctly refused.
func TestExpandedRepliesTruncateAndAreResumable(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)

	replies := make([]teams.ChatMessage, graphmock.TeamsExpandedReplyCap+3)
	for i := range replies {
		replies[i] = teams.ChatMessage{ID: "early", MessageType: teams.MessageTypeMessage, ReplyToID: "1"}
	}
	server.Handle("GET /teams/{team}/channels/{channel}/messages", func(_ *graphmock.Request) graphmock.Response {
		return graphmock.Collection([]teams.ChatMessage{
			{ID: "1", MessageType: teams.MessageTypeMessage, Replies: replies},
		}, "")
	})
	server.Handle("GET /teams/{team}/channels/{channel}/messages/{message}/replies",
		func(_ *graphmock.Request) graphmock.Response {
			return graphmock.Collection([]teams.ChatMessage{
				{ID: "tail", MessageType: teams.MessageTypeMessage, ReplyToID: "1"},
			}, "")
		})
	client := newTeamsClient(t, server)

	var root teams.ChatMessage
	for message, err := range client.ChannelMessages(context.Background(), "t", "19:c@thread.tacv2",
		teams.ChannelMessageQuery{WithReplies: true}) {
		if err != nil {
			t.Fatal(err)
		}
		root = message
	}
	if got := len(root.Replies); got != graphmock.TeamsExpandedReplyCap {
		t.Fatalf("expanded replies = %d, want them truncated at %d", got, graphmock.TeamsExpandedReplyCap)
	}
	if root.RepliesNextLink == "" {
		t.Fatal("a truncated thread carried no continuation link, so nothing tells a caller it stopped early")
	}

	var tail []teams.MessageID
	for reply, err := range client.MoreReplies(context.Background(), root) {
		if err != nil {
			t.Fatalf("following the link a truncated thread carried: %v", err)
		}
		tail = append(tail, reply.ID)
	}
	if len(tail) != 1 || tail[0] != "tail" {
		t.Fatalf("resuming the thread returned %v, want the remainder", tail)
	}
}

// TestMoreRepliesFollowsTheNestedLink pins that MoreReplies actually
// dereferences [teams.ChatMessage.RepliesNextLink] rather than treating it as
// inert metadata, and that a message with no link yields nothing rather than
// an error.
func TestMoreRepliesFollowsTheNestedLink(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	var followedPath string
	server.Handle("GET /teamsFake/messages/{message}/replies", func(r *graphmock.Request) graphmock.Response {
		followedPath = r.Path
		return graphmock.Collection([]teams.ChatMessage{
			{ID: "r1", MessageType: teams.MessageTypeMessage, ReplyToID: "1"},
		}, "")
	})
	client := newTeamsClient(t, server)

	link := server.Link("/teamsFake/messages/1/replies", "$skiptoken=graphmock")
	message := teams.ChatMessage{ID: "1", RepliesNextLink: link}

	var got []teams.MessageID
	for reply, err := range client.MoreReplies(context.Background(), message) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, reply.ID)
	}
	if len(got) != 1 || got[0] != "r1" {
		t.Fatalf("MoreReplies returned %v, want the replies from the followed link", got)
	}
	if followedPath == "" {
		t.Fatal("MoreReplies never reached the nested link")
	}

	var count int
	for _, err := range client.MoreReplies(context.Background(), teams.ChatMessage{}) {
		count++
		if err != nil {
			t.Fatal(err)
		}
	}
	if count != 0 {
		t.Fatalf("MoreReplies with no RepliesNextLink yielded %d items, want 0 and no error", count)
	}
}
