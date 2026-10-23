# teams-go

`teams-go` is a typed Go client for Microsoft Teams over Microsoft Graph. It owns Teams request shapes, identifiers, message models, and permission hints. `msgraph-go` owns HTTP, paging, retries, and Graph errors; callers supply authentication.

## Use

```go
graph, err := msgraph.New(auth.TokenSource("Chat.Read"))
if err != nil { return err }
client, err := teams.New(graph)
if err != nil { return err }

for message, err := range client.Messages(ctx, chatID, teams.MessageQuery{Top: 50}) {
    if err != nil { return err }
    fmt.Println(message.PlainText())
}
```

`auth` above can be an `msauth-go.Authenticator`; any `msgraph.TokenSource` works. Teams Graph endpoints require a work or school account.

## Current API

| Area | Operations |
| --- | --- |
| Chats | List and get chats; list members; read and send messages; react; soft-delete and restore messages |
| Teams and channels | List joined teams and channels; read root messages and replies; post root messages and replies; list channel members |
| Search and presence | Search messages; read own or another user's presence |

Channel message listings contain roots only and are ordered by the latest change in each entire thread. `ChannelMessageQuery` supports a page size and optional reply expansion; `ReplyQuery` supports a page size. `ChannelMessageQuery.WithReplies` includes the first page of each thread; follow `Client.MoreReplies` when `RepliesNextLink` is set. Chat `MessageQuery` couples the filter and sort property because Graph otherwise silently discards mismatched filters. Its inclusive `Since` bound is applied while paging, since Graph only supports exclusive upper bounds on the chat message timestamps.

When upgrading from v0.1.1, replace `ChannelMessageQuery{MessageQuery: MessageQuery{Top: n}}` with `ChannelMessageQuery{Top: n}` and pass `ReplyQuery` to `Client.Replies`. Channel endpoints cannot use chat date filters or sorting.

Use `Capabilities`, `ScopesFor`, and `NeedsAdminConsent` before asking users for permissions. Several team, channel, and presence reads need tenant administrator consent. The SDK does not perform sign-in or grant permissions.

The SDK does not currently create chats or teams, manage memberships, download hosted content, or model meetings, calls, tabs, or Shifts. These are separate Graph areas with different contracts and permissions.

## Develop

```sh
just ci
```

Tests use `msgraph-go/graphmock` to exercise service constraints and response shapes. A live tenant check remains necessary before claiming full service compatibility.
