package teams_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	teams "github.com/albertocavalcante/teams-go"
)

// TestEmptyIdentifiersAreRefusedBeforeARequest pins that an empty ChatID,
// TeamID, ChannelID, MessageID, or UserID is refused while the request is
// being built, never sent to the service as a malformed path. No handler is
// registered on the fake at all: if any of these reached it, the request
// would panic the test for hitting an unregistered route, which is exactly
// the point -- these must never get that far.
func TestEmptyIdentifiersAreRefusedBeforeARequest(t *testing.T) {
	t.Parallel()
	server := newTeamsServer(t)
	client := newTeamsClient(t, server)
	ctx := context.Background()

	draft := teams.Draft{Body: teams.TextBody("hi")}

	checks := []struct {
		name string
		err  error
	}{
		{"Messages with empty chat", firstIterErr(client.Messages(ctx, "", teams.MessageQuery{}))},
		{"GetMessage with empty chat", errFrom3(client.GetMessage(ctx, "", "m1"))},
		{"GetMessage with empty message", errFrom3(client.GetMessage(ctx, "c1", ""))},
		{"SendMessage with empty chat", errFrom3(client.SendMessage(ctx, "", draft))},
		{"SetReaction with empty chat", errFrom2(client.SetReaction(ctx, "", "m1", teams.ReactionLike))},
		{"SetReaction with empty message", errFrom2(client.SetReaction(ctx, "c1", "", teams.ReactionLike))},
		{"UnsetReaction with empty chat", errFrom2(client.UnsetReaction(ctx, "", "m1", teams.ReactionLike))},
		{"SoftDeleteMessage with empty chat", errFrom2(client.SoftDeleteMessage(ctx, "", "m1"))},
		{"UndoSoftDeleteMessage with empty chat", errFrom2(client.UndoSoftDeleteMessage(ctx, "", "m1"))},
		{"GetChat with empty chat", errFrom3(client.GetChat(ctx, "", teams.ChatQuery{}))},
		{"ChatMembers with empty chat", firstIterErr(client.ChatMembers(ctx, ""))},
		{"GetChannel with empty team", errFrom3(client.GetChannel(ctx, "", "ch1"))},
		{"GetChannel with empty channel", errFrom3(client.GetChannel(ctx, "t1", ""))},
		{"Channels with empty team", firstIterErr(client.Channels(ctx, "", teams.ChannelQuery{}))},
		{"ChannelMessages with empty team", firstIterErr(client.ChannelMessages(ctx, "", "ch1", teams.ChannelMessageQuery{}))},
		{"ChannelMessages with empty channel", firstIterErr(client.ChannelMessages(ctx, "t1", "", teams.ChannelMessageQuery{}))},
		{"Replies with empty message", firstIterErr(client.Replies(ctx, "t1", "ch1", "", teams.MessageQuery{}))},
		{"SendChannelMessage with empty channel", errFrom3(client.SendChannelMessage(ctx, "t1", "", draft))},
		{"ReplyToChannelMessage with empty message", errFrom3(client.ReplyToChannelMessage(ctx, "t1", "ch1", "", draft))},
		{"ChannelMembers with empty channel", firstIterErr(client.ChannelMembers(ctx, "t1", ""))},
		{"UserPresence with empty user", errFrom3(client.UserPresence(ctx, ""))},
		{"GetTeam with empty team", errFrom3(client.GetTeam(ctx, ""))},
	}
	for _, c := range checks {
		if !errors.Is(c.err, teams.ErrEmptyID) {
			t.Errorf("%s: err = %v, want ErrEmptyID", c.name, c.err)
		}
	}

	if got := len(server.Requests()); got != 0 {
		t.Fatalf("empty identifiers must be refused before a request, but %d reached the fake: %v", got, server.Requests())
	}
}

func firstIterErr[T any](seq iter.Seq2[T, error]) error {
	var got error
	for _, err := range seq {
		got = err
		break
	}
	return got
}

func errFrom2(_ any, err error) error { return err }

func errFrom3(_, _ any, err error) error { return err }
