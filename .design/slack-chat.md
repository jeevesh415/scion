# Slack Chat Provider for Scion Chat App

**Created:** 2026-04-08
**Status:** Draft
**Related:** `.design/google-chat.md`, `.design/message-broker-plugin-evolution.md`, `.design/chat-plugin-tradeoffs.md`

---

## Overview

This design describes the Slack adapter implementation for the `scion-chat-app`. It is the second platform provider, following the Google Chat adapter that shipped in Phases 1 and 2. Both adapters share the same core engine, command router, notification relay, identity system, broker plugin, and SQLite state layer. The Slack adapter only needs to implement the `Messenger` interface and provide event ingestion, translating between Slack's APIs and the platform-agnostic abstractions already in place.

### Goals

- Implement `chatapp.Messenger` for Slack using the Slack Web API and Block Kit
- Receive events via Slack's Events API (HTTP) and Socket Mode (WebSocket)
- Render the existing `Card` / `Dialog` / `Widget` model as Slack Block Kit
- Leverage Slack's dynamic bot identity to show per-agent personas (name + avatar)
- Use Slack Modals (`views.open` / `views.update`) for the `Dialog` abstraction
- Support native Slack threading via `thread_ts`
- Provide an App Home tab as a grove dashboard
- Integrate with the existing command router, notification relay, and identity mapper without changes to shared code

### Non-Goals (This Phase)

- Slack Enterprise Grid (multi-workspace) support
- Slack Connect (cross-org channels)
- Unfurling of Scion Hub URLs in Slack messages
- File/attachment relay to agents
- Workflow Builder integration

---

## Architecture

### Where the Slack Adapter Fits

The Slack adapter slots into the existing architecture at the same layer as the Google Chat adapter. Both are interchangeable implementations of `chatapp.Messenger` that normalize platform events to `ChatEvent` and render `Card`/`Dialog` types to platform-native formats.

```
                    ┌─────────────────────────────────────────┐
                    │            Slack Platform               │
                    │  Events API / Socket Mode / Web API     │
                    └──────────┬──────────────────────────────┘
                               │
                    ┌──────────▼──────────────────────────────┐
                    │     internal/slack/adapter.go            │
                    │     (implements chatapp.Messenger)       │
                    │                                         │
                    │  ┌────────────┐  ┌──────────────────┐   │
                    │  │ Event      │  │ Block Kit         │   │
                    │  │ Normalizer │  │ Renderer          │   │
                    │  └──────┬─────┘  └────────┬─────────┘   │
                    │         │                  │             │
                    └─────────┼──────────────────┼─────────────┘
                              │                  │
                  ┌───────────▼──────────────────▼────────────┐
                  │         Shared Core Engine                │
                  │  CommandRouter · NotificationRelay        │
                  │  IdentityMapper · StateStore · Broker     │
                  └───────────────────────────────────────────┘
```

Both adapters can run simultaneously in the same process. The existing `PlatformsConfig` already contains a `Slack SlackConfig` field; when `slack.enabled: true`, `main.go` creates the Slack adapter alongside (or instead of) the Google Chat adapter, wiring it to the same `CommandRouter`.

### Module & Package Layout

All Slack-specific code lives in a single new package:

```
extras/scion-chat-app/
├── internal/
│   ├── slack/
│   │   ├── adapter.go        # SlackAdapter: Messenger impl, event server
│   │   ├── blocks.go         # Card → Block Kit rendering
│   │   ├── events.go         # Event normalization (Slack → ChatEvent)
│   │   ├── modals.go         # Dialog → Slack Modal rendering
│   │   ├── apphome.go        # App Home tab rendering
│   │   ├── verify.go         # Request signature verification
│   │   └── adapter_test.go   # Unit tests
│   ├── chatapp/              # (unchanged)
│   ├── googlechat/           # (unchanged)
│   ├── identity/             # (unchanged)
│   └── state/                # (unchanged)
```

### Dependencies

The adapter uses the official Slack SDK for Go:

```
github.com/slack-go/slack
```

This is added to the `extras/scion-chat-app/go.mod` only, keeping the Slack SDK out of the main Scion module's dependency tree (same pattern as the Google Chat SDK).

---

## Slack App Configuration

### Required Slack App Manifest

The Slack app is created and configured in the [Slack API dashboard](https://api.slack.com/apps). The manifest defines scopes, event subscriptions, slash commands, and interactivity endpoints. A representative manifest:

```yaml
display_information:
  name: Scion
  description: Scion agent management for Slack
  background_color: "#1a1a2e"

features:
  app_home:
    home_tab_enabled: true
    messages_tab_enabled: false
  bot_user:
    display_name: Scion
    always_online: true
  slash_commands:
    - command: /scion
      url: https://chat.example.com/slack/commands
      description: Scion agent management
      usage_hint: "[list|status|start|stop|create|delete|logs|message|link|unlink|register|unregister|subscribe|unsubscribe|info|help]"

oauth_config:
  scopes:
    bot:
      - channels:read
      - chat:write
      - chat:write.customize    # Required for per-agent username/avatar
      - commands
      - im:read
      - im:write
      - users:read
      - users:read.email        # Required for email-based identity mapping
      # groups:read and mpim:read omitted — private channels deferred to a future phase

settings:
  event_subscriptions:
    bot_events:
      - app_home_opened
      - app_mention
      - member_joined_channel
      - member_left_channel
      - message.im              # DMs to the bot
    request_url: https://chat.example.com/slack/events
  interactivity:
    is_enabled: true
    request_url: https://chat.example.com/slack/interactions
  socket_mode_enabled: false    # true for local dev / private network deployments
```

### Bot Token vs. Socket Mode

The adapter supports two connection modes:

| Mode | When to use | How it works |
|------|-------------|--------------|
| **HTTP (Events API)** | Production with a public endpoint | Slack POSTs events to the configured request URL |
| **Socket Mode** | Development, private networks, no public endpoint | App opens a WebSocket to Slack's servers; no inbound HTTP needed |

Both modes produce identical event payloads. The adapter abstracts this behind a single event receiver that feeds `normalizeEvent()`.

**Configuration:**

```yaml
platforms:
  slack:
    enabled: true
    bot_token: "${SLACK_BOT_TOKEN}"          # xoxb-...
    app_token: "${SLACK_APP_TOKEN}"          # xapp-... (Socket Mode only)
    signing_secret: "${SLACK_SIGNING_SECRET}" # HTTP mode only
    listen_address: ":8444"                  # HTTP mode only
    socket_mode: false                       # true to use Socket Mode
```

---

## Adapter Implementation

### SlackAdapter Structure

```go
package slack

import (
    "context"
    "log/slog"
    "net/http"

    slackapi "github.com/slack-go/slack"
    "github.com/slack-go/slack/socketmode"

    "github.com/GoogleCloudPlatform/scion/extras/scion-chat-app/internal/chatapp"
)

const PlatformName = "slack"

// EventHandler processes normalized chat events.
type EventHandler func(ctx context.Context, event *chatapp.ChatEvent) (*chatapp.EventResponse, error)

// Config holds Slack adapter configuration.
type Config struct {
    BotToken      string // xoxb-...
    AppToken      string // xapp-... (Socket Mode)
    SigningSecret string // Request signing secret (HTTP mode)
    ListenAddress string // HTTP listen address (HTTP mode)
    SocketMode    bool   // Use Socket Mode instead of HTTP
}

// Adapter implements chatapp.Messenger for Slack.
type Adapter struct {
    client       *slackapi.Client
    socketClient *socketmode.Client // nil when not using Socket Mode
    signingSecret string
    httpServer   *http.Server
    handler      EventHandler
    log          *slog.Logger
}
```

### Messenger Interface Mapping

Each `Messenger` method maps to specific Slack Web API calls:

| Messenger Method | Slack API | Notes |
|-----------------|-----------|-------|
| `SendMessage()` | `chat.postMessage` | Supports `username`/`icon_url` overrides via `chat:write.customize` |
| `SendCard()` | `chat.postMessage` with blocks | Card rendered as Block Kit attachment |
| `UpdateMessage()` | `chat.update` | Uses message `ts` as the message ID |
| `OpenDialog()` | `views.open` | Converts `Dialog` to Slack Modal view |
| `UpdateDialog()` | `views.update` | Updates an open modal by `view_id` |
| `GetUser()` | `users.info` | Returns Slack user profile including email |
| `SetAgentIdentity()` | _(no-op, identity set per-message)_ | Identity applied at send time via `chat:write.customize` |

#### SendMessage

```go
func (a *Adapter) SendMessage(ctx context.Context, req chatapp.SendMessageRequest) (string, error) {
    opts := []slackapi.MsgOption{
        slackapi.MsgOptionText(req.Text, false),
    }

    // Per-agent identity override
    if req.AgentID != "" {
        opts = append(opts,
            slackapi.MsgOptionUsername(req.AgentID),
            slackapi.MsgOptionIconURL(agentIconURL(req.AgentID)),
        )
    }

    // Threading
    if req.ThreadID != "" {
        opts = append(opts, slackapi.MsgOptionTS(req.ThreadID))
    }

    // Block Kit cards
    if req.Card != nil {
        blocks := renderBlocks(req.Card)
        opts = append(opts, slackapi.MsgOptionBlocks(blocks...))
    }

    _, ts, err := a.client.PostMessageContext(ctx, req.SpaceID, opts...)
    return ts, err
}
```

The return value is the message timestamp (`ts`), which serves as the message ID in Slack. This is stored as the `ThreadID` / message reference for updates and threading.

#### SendCard

```go
func (a *Adapter) SendCard(ctx context.Context, spaceID string, card chatapp.Card) (string, error) {
    return a.SendMessage(ctx, chatapp.SendMessageRequest{
        SpaceID: spaceID,
        Card:    &card,
    })
}
```

#### UpdateMessage

```go
func (a *Adapter) UpdateMessage(ctx context.Context, messageID string, req chatapp.SendMessageRequest) error {
    // messageID is the Slack message timestamp (ts)
    // Channel is extracted from the message ID or stored alongside it
    opts := []slackapi.MsgOption{
        slackapi.MsgOptionText(req.Text, false),
    }
    if req.Card != nil {
        blocks := renderBlocks(req.Card)
        opts = append(opts, slackapi.MsgOptionBlocks(blocks...))
    }

    _, _, _, err := a.client.UpdateMessageContext(ctx, req.SpaceID, messageID, opts...)
    return err
}
```

#### OpenDialog / UpdateDialog

Slack Modals are opened via `views.open` using a `trigger_id` from the originating interaction:

```go
func (a *Adapter) OpenDialog(ctx context.Context, triggerID string, dialog chatapp.Dialog) error {
    view := renderModal(&dialog)
    _, err := a.client.OpenViewContext(ctx, triggerID, view)
    return err
}

func (a *Adapter) UpdateDialog(ctx context.Context, viewID string, dialog chatapp.Dialog) error {
    view := renderModal(&dialog)
    _, err := a.client.UpdateViewContext(ctx, view, "", "", viewID)
    return err
}
```

#### GetUser

```go
func (a *Adapter) GetUser(ctx context.Context, userID string) (*chatapp.ChatUser, error) {
    user, err := a.client.GetUserInfoContext(ctx, userID)
    if err != nil {
        return nil, err
    }
    return &chatapp.ChatUser{
        PlatformID:  user.ID,
        DisplayName: user.Profile.DisplayName,
        Email:       user.Profile.Email,
    }, nil
}
```

This is a key difference from Google Chat: Google Chat provides the user's email in every signed webhook payload (`event.UserEmail`), so no separate API call is needed. Slack does not include email in event payloads, so the adapter must call `users.info` to retrieve it. Results should be cached to avoid rate limits.

#### SetAgentIdentity

```go
func (a *Adapter) SetAgentIdentity(ctx context.Context, agent chatapp.AgentIdentity) error {
    // No-op: Slack agent identity is applied per-message via chat:write.customize
    return nil
}
```

---

## Event Handling

### Event Ingestion

The adapter receives events through three Slack API surfaces, all served from the same HTTP server (or Socket Mode client):

| Endpoint | Slack API Surface | Event Types |
|----------|-------------------|-------------|
| `POST /slack/events` | Events API | `app_mention`, `message.im`, `member_joined_channel`, `member_left_channel`, `app_home_opened` |
| `POST /slack/commands` | Slash Commands | `/scion` with all subcommands |
| `POST /slack/interactions` | Interactivity | Button clicks, modal submissions, overflow menus |

#### HTTP Server

```go
func (a *Adapter) Start(listenAddr string) error {
    mux := http.NewServeMux()
    mux.HandleFunc("POST /slack/events", a.handleEvents)
    mux.HandleFunc("POST /slack/commands", a.handleCommands)
    mux.HandleFunc("POST /slack/interactions", a.handleInteractions)
    mux.HandleFunc("GET /slack/healthz", a.handleHealthz)

    a.httpServer = &http.Server{Addr: listenAddr, Handler: mux}
    return a.httpServer.ListenAndServe()
}
```

#### Socket Mode Alternative

When `socket_mode: true`, the adapter starts a WebSocket connection instead of an HTTP server:

```go
func (a *Adapter) StartSocketMode() error {
    a.socketClient = socketmode.New(a.client,
        socketmode.OptionAppLevelToken(a.appToken),
    )

    go func() {
        for evt := range a.socketClient.Events {
            switch evt.Type {
            case socketmode.EventTypeEventsAPI:
                a.handleSocketEvent(evt)
            case socketmode.EventTypeSlashCommand:
                a.handleSocketCommand(evt)
            case socketmode.EventTypeInteractive:
                a.handleSocketInteraction(evt)
            }
        }
    }()

    return a.socketClient.Run()
}
```

Socket Mode requires an **App-Level Token** (`xapp-...`) in addition to the Bot Token. This is generated in the Slack app settings under "Basic Information > App-Level Tokens" with the `connections:write` scope.

### Request Verification (HTTP Mode)

All inbound HTTP requests are verified using Slack's signing secret before processing:

```go
func (a *Adapter) verifyRequest(r *http.Request, body []byte) error {
    sv, err := slackapi.NewSecretsVerifier(r.Header, a.signingSecret)
    if err != nil {
        return err
    }
    if _, err := sv.Write(body); err != nil {
        return err
    }
    return sv.Ensure()
}
```

This replaces Google Chat's JWT-based verification. The signing secret is a shared secret configured in the Slack app settings.

### Event Normalization

All Slack events are normalized to `chatapp.ChatEvent` before being dispatched to the shared `CommandRouter`:

```go
func (a *Adapter) normalizeSlashCommand(cmd slackapi.SlashCommand) *chatapp.ChatEvent {
    return &chatapp.ChatEvent{
        Type:     chatapp.EventCommand,
        Platform: PlatformName,
        SpaceID:  cmd.ChannelID,
        UserID:   cmd.UserID,
        Command:  "scion",
        Args:     cmd.Text,
    }
}

func (a *Adapter) normalizeAppMention(evt *slackapi.AppMentionEvent) *chatapp.ChatEvent {
    // Strip the bot mention prefix (e.g., "<@U0ABC123> tell deploy-agent...")
    text := stripBotMention(evt.Text)
    return &chatapp.ChatEvent{
        Type:     chatapp.EventMessage,
        Platform: PlatformName,
        SpaceID:  evt.Channel,
        ThreadID: evt.ThreadTimeStamp, // thread_ts if in a thread, else ""
        UserID:   evt.User,
        Text:     text,
    }
}

func (a *Adapter) normalizeInteraction(callback slackapi.InteractionCallback) *chatapp.ChatEvent {
    event := &chatapp.ChatEvent{
        Platform: PlatformName,
        SpaceID:  callback.Channel.ID,
        UserID:   callback.User.ID,
    }

    switch callback.Type {
    case slackapi.InteractionTypeBlockActions:
        action := callback.ActionCallback.BlockActions[0]
        event.Type = chatapp.EventAction
        event.ActionID = action.ActionID
        event.ActionData = action.Value
        if callback.Message.ThreadTimestamp != "" {
            event.ThreadID = callback.Message.ThreadTimestamp
        } else {
            event.ThreadID = callback.Message.Timestamp
        }

    case slackapi.InteractionTypeViewSubmission:
        event.Type = chatapp.EventDialogSubmit
        event.ActionID = callback.View.CallbackID
        event.DialogData = extractModalValues(callback.View.State)
        // For modal submissions, we use the private_metadata to carry
        // the originating channel ID since modals are channel-independent
        event.SpaceID = callback.View.PrivateMetadata
    }

    return event
}
```

**Key normalization differences from Google Chat:**

| Aspect | Google Chat | Slack |
|--------|------------|-------|
| User email | Included in every event payload | Must call `users.info` API separately |
| Thread ID | `thread.name` (resource path) | `thread_ts` (message timestamp) |
| Space ID | `space.name` (resource path, e.g., `spaces/ABC`) | Channel ID (e.g., `C0ABC123`) |
| Action callbacks | Parameters in `commonEventObject` | `action_id` + `value` in block action |
| Dialog submissions | Form inputs in `commonEventObject.formInputs` | View state values in `view.state.values` |
| Slash command args | `message.argumentText` | `text` field of slash command payload |
| Bot added to channel | `addedToSpacePayload` | `member_joined_channel` event (for bot user) |
| Bot removed | `removedFromSpacePayload` | `member_left_channel` event (for bot user) |

### Synchronous vs. Asynchronous Responses

This is the most significant architectural difference between the two adapters.

**Google Chat** returns synchronous JSON responses in the HTTP body. The `EventResponse` type was designed for this: command handlers return `EventResponse` objects, and the Google Chat adapter serializes them into the webhook response.

**Slack** requires acknowledging events within 3 seconds with a `200 OK` (optionally with a short text response for slash commands). Any rich content must be sent asynchronously via the Web API.

The adapter bridges this gap:

```go
func (a *Adapter) handleCommands(w http.ResponseWriter, r *http.Request) {
    // 1. Verify signature
    // 2. Parse slash command
    cmd, _ := slackapi.SlashCommandParse(r)

    // 3. Acknowledge immediately with an ephemeral "Processing..." message
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]any{
        "response_type": "ephemeral",
        "text":          "Processing...",
    })

    // 4. Process command asynchronously
    go func() {
        event := a.normalizeSlashCommand(cmd)
        resp, err := a.handler(context.Background(), event)
        if err != nil {
            a.log.Error("command handler error", "error", err)
            return
        }
        // 5. Send the actual response via Web API
        if resp != nil && resp.Message != nil {
            a.sendResponse(context.Background(), cmd.ChannelID, resp)
        }
    }()
}
```

For **interactions** (button clicks, modal submissions), the same pattern applies: acknowledge immediately, then call the Web API to post the response. Modal opens are the exception: `views.open` must be called within 3 seconds of the interaction using the `trigger_id`.

The `EventResponse` type maps to Slack responses as follows:

| EventResponse field | Slack behavior |
|--------------------|---------------|
| `Message` | `chat.postMessage` to the originating channel |
| `UpdateMessage` | `chat.update` on the originating message |
| `Dialog` | `views.open` with the `trigger_id` from the interaction |
| `CloseDialog` | Return empty `response_action` (modal auto-closes on submit) |
| `Notification` | Ephemeral message via `chat.postEphemeral` |

---

## Block Kit Rendering

### Card to Block Kit Mapping

The platform-agnostic `Card` model maps to Slack Block Kit as follows:

| Card Element | Block Kit Element |
|-------------|-------------------|
| `CardHeader` | `header` block + `context` block (for subtitle) |
| `CardSection` with header | `section` block with `text` (section header as bold text) |
| `WidgetText` | `section` block with `mrkdwn` text |
| `WidgetKeyValue` | `section` block with `fields` (label bold, value plain) |
| `WidgetButton` | `actions` block with `button` element |
| `WidgetDivider` | `divider` block |
| `WidgetImage` | `image` block |
| `WidgetInput` | `input` block with `plain_text_input` element |
| `WidgetCheckbox` | `input` block with `checkboxes` element |
| `CardAction` list | `actions` block with button elements |

### Rendering Implementation

```go
// blocks.go

func renderBlocks(card *chatapp.Card) []slackapi.Block {
    var blocks []slackapi.Block

    // Header
    if card.Header.Title != "" {
        blocks = append(blocks, slackapi.NewHeaderBlock(
            slackapi.NewTextBlockObject("plain_text", card.Header.Title, false, false),
        ))
        if card.Header.Subtitle != "" {
            blocks = append(blocks, slackapi.NewContextBlock("",
                slackapi.NewTextBlockObject("mrkdwn", card.Header.Subtitle, false, false),
            ))
        }
    }

    // Sections
    for _, section := range card.Sections {
        if section.Header != "" {
            blocks = append(blocks, slackapi.NewSectionBlock(
                slackapi.NewTextBlockObject("mrkdwn", "*"+section.Header+"*", false, false),
                nil, nil,
            ))
        }
        for _, widget := range section.Widgets {
            blocks = append(blocks, renderWidget(&widget)...)
        }
    }

    // Card-level actions as a single actions block
    if len(card.Actions) > 0 {
        var buttons []slackapi.BlockElement
        for _, action := range card.Actions {
            btn := slackapi.NewButtonBlockElement(
                action.ActionID,
                action.ActionID,
                slackapi.NewTextBlockObject("plain_text", action.Label, false, false),
            )
            switch action.Style {
            case "primary":
                btn.Style = slackapi.StylePrimary
            case "danger":
                btn.Style = slackapi.StyleDanger
            }
            buttons = append(buttons, btn)
        }
        blocks = append(blocks, slackapi.NewActionBlock("", buttons...))
    }

    return blocks
}

func renderWidget(w *chatapp.Widget) []slackapi.Block {
    switch w.Type {
    case chatapp.WidgetText:
        return []slackapi.Block{
            slackapi.NewSectionBlock(
                slackapi.NewTextBlockObject("mrkdwn", w.Content, false, false),
                nil, nil,
            ),
        }

    case chatapp.WidgetKeyValue:
        return []slackapi.Block{
            slackapi.NewSectionBlock(nil,
                []*slackapi.TextBlockObject{
                    slackapi.NewTextBlockObject("mrkdwn", "*"+w.Label+"*", false, false),
                    slackapi.NewTextBlockObject("mrkdwn", w.Content, false, false),
                },
                nil,
            ),
        }

    case chatapp.WidgetButton:
        btn := slackapi.NewButtonBlockElement(
            w.ActionID, w.ActionData,
            slackapi.NewTextBlockObject("plain_text", w.Label, false, false),
        )
        return []slackapi.Block{slackapi.NewActionBlock("", btn)}

    case chatapp.WidgetDivider:
        return []slackapi.Block{slackapi.NewDividerBlock()}

    case chatapp.WidgetImage:
        return []slackapi.Block{
            slackapi.NewImageBlock(w.Content, w.Label, "", nil),
        }

    case chatapp.WidgetInput:
        input := slackapi.NewPlainTextInputBlockElement(
            slackapi.NewTextBlockObject("plain_text", w.Label, false, false),
            w.ActionID,
        )
        return []slackapi.Block{
            slackapi.NewInputBlock(w.ActionID, slackapi.NewTextBlockObject("plain_text", w.Label, false, false), nil, input),
        }

    case chatapp.WidgetCheckbox:
        var options []*slackapi.OptionBlockObject
        for _, opt := range w.Options {
            options = append(options, slackapi.NewOptionBlockObject(
                opt.Value,
                slackapi.NewTextBlockObject("plain_text", opt.Label, false, false),
                nil,
            ))
        }
        checkboxes := slackapi.NewCheckboxGroupsBlockElement(w.ActionID, options...)
        return []slackapi.Block{
            slackapi.NewInputBlock(w.ActionID, slackapi.NewTextBlockObject("plain_text", w.Label, false, false), nil, checkboxes),
        }

    default:
        return nil
    }
}
```

### Block Kit Limits

Slack imposes specific limits on Block Kit messages:

| Limit | Value | Mitigation |
|-------|-------|------------|
| Blocks per message | 50 | Truncate long agent lists; paginate logs |
| Text block length | 3000 chars | Truncate log output (already done at 2000 chars) |
| Actions per block | 25 | Unlikely to hit; card actions are typically 2-4 |
| Fields per section | 10 | Agent status cards typically use 4-5 fields |
| Attachments per message | 20 | Only one card per message in current design |

The existing 2000-char truncation for log output in `cmdLogs()` already satisfies the text block limit. No changes to the command router are needed.

---

## Modal Rendering (Dialogs)

The `chatapp.Dialog` type maps to Slack Modals:

```go
// modals.go

func renderModal(dialog *chatapp.Dialog) slackapi.ModalViewRequest {
    var blocks slackapi.Blocks

    for _, field := range dialog.Fields {
        switch field.Type {
        case "text":
            input := slackapi.NewPlainTextInputBlockElement(
                slackapi.NewTextBlockObject("plain_text", field.Placeholder, false, false),
                field.ID,
            )
            block := slackapi.NewInputBlock(
                field.ID,
                slackapi.NewTextBlockObject("plain_text", field.Label, false, false),
                nil,
                input,
            )
            block.Optional = !field.Required
            blocks.BlockSet = append(blocks.BlockSet, block)

        case "textarea":
            input := slackapi.NewPlainTextInputBlockElement(
                slackapi.NewTextBlockObject("plain_text", field.Placeholder, false, false),
                field.ID,
            )
            input.Multiline = true
            block := slackapi.NewInputBlock(
                field.ID,
                slackapi.NewTextBlockObject("plain_text", field.Label, false, false),
                nil,
                input,
            )
            block.Optional = !field.Required
            blocks.BlockSet = append(blocks.BlockSet, block)

        case "select":
            var options []*slackapi.OptionBlockObject
            for _, opt := range field.Options {
                options = append(options, slackapi.NewOptionBlockObject(
                    opt.Value,
                    slackapi.NewTextBlockObject("plain_text", opt.Label, false, false),
                    nil,
                ))
            }
            sel := slackapi.NewOptionsSelectBlockElement(
                "static_select", nil, field.ID, options...,
            )
            block := slackapi.NewInputBlock(
                field.ID,
                slackapi.NewTextBlockObject("plain_text", field.Label, false, false),
                nil,
                sel,
            )
            block.Optional = !field.Required
            blocks.BlockSet = append(blocks.BlockSet, block)

        case "checkbox":
            var options []*slackapi.OptionBlockObject
            for _, opt := range field.Options {
                options = append(options, slackapi.NewOptionBlockObject(
                    opt.Value,
                    slackapi.NewTextBlockObject("plain_text", opt.Label, false, false),
                    nil,
                ))
            }
            cb := slackapi.NewCheckboxGroupsBlockElement(field.ID, options...)
            block := slackapi.NewInputBlock(
                field.ID,
                slackapi.NewTextBlockObject("plain_text", field.Label, false, false),
                nil,
                cb,
            )
            block.Optional = !field.Required
            blocks.BlockSet = append(blocks.BlockSet, block)
        }
    }

    return slackapi.ModalViewRequest{
        Type:            "modal",
        Title:           slackapi.NewTextBlockObject("plain_text", dialog.Title, false, false),
        Submit:          slackapi.NewTextBlockObject("plain_text", dialog.Submit.Label, false, false),
        Close:           slackapi.NewTextBlockObject("plain_text", dialog.Cancel.Label, false, false),
        CallbackID:      dialog.Submit.ActionID,
        Blocks:          blocks,
    }
}
```

### Modal Value Extraction

When a modal is submitted, Slack sends the form values in a nested `view.state.values` structure. The adapter flattens this into the `ChatEvent.DialogData` map expected by the `CommandRouter`:

```go
func extractModalValues(state *slackapi.ViewState) map[string]string {
    result := make(map[string]string)
    for blockID, blockValues := range state.Values {
        for actionID, action := range blockValues {
            switch action.Type {
            case "plain_text_input":
                result[actionID] = action.Value
            case "static_select":
                result[actionID] = action.SelectedOption.Value
            case "checkboxes":
                var vals []string
                for _, opt := range action.SelectedOptions {
                    vals = append(vals, opt.Value)
                }
                result[actionID] = strings.Join(vals, ",")
            }
            // Also store under blockID for compatibility
            if _, exists := result[blockID]; !exists {
                result[blockID] = result[actionID]
            }
        }
    }
    return result
}
```

---

## Dynamic Agent Identity

This is a major advantage Slack has over Google Chat. Google Chat fixes the bot name and avatar at the app configuration level, requiring card headers as a workaround to distinguish agents. Slack's `chat:write.customize` scope allows overriding `username` and `icon_url` per message.

### Per-Agent Persona

When sending messages on behalf of an agent, the adapter applies the agent's identity:

```go
func (a *Adapter) SendMessage(ctx context.Context, req chatapp.SendMessageRequest) (string, error) {
    opts := []slackapi.MsgOption{
        slackapi.MsgOptionText(req.Text, false),
    }

    if req.AgentID != "" {
        opts = append(opts,
            slackapi.MsgOptionUsername(req.AgentID),
            slackapi.MsgOptionIconURL(agentIconURL(req.AgentID)),
        )
    }
    // ...
}
```

This makes each agent appear as a distinct "user" in the channel — with its own name and avatar — rather than all messages coming from a single "Scion" bot.

#### Agent Icon Generation

Agent avatars are generated via [RoboHash](https://robohash.org/), which produces deterministic robot-themed images from any input string. The agent slug is used as the seed, so the same agent always gets the same avatar across sessions and channels.

The implementation is behind an `IconProvider` abstraction so the source can be swapped later (e.g., to a Hub-managed avatar endpoint or a different generator):

```go
// IconProvider generates avatar URLs for agents.
type IconProvider interface {
    IconURL(agentSlug string) string
}

// robohashProvider generates avatars via robohash.org.
type robohashProvider struct{}

func (r *robohashProvider) IconURL(agentSlug string) string {
    return fmt.Sprintf("https://robohash.org/%s?set=set1&size=48x48", url.PathEscape(agentSlug))
}
```

The adapter takes an `IconProvider` at construction time, defaulting to `robohashProvider`.

### Identity in Notifications

The `NotificationRelay` already sets `AgentID` on `SendMessageRequest` when relaying agent messages. For Google Chat, this is ignored (identity is in the card header). For Slack, the adapter picks it up and applies it as the bot username/avatar.

This means notification cards from `deploy-agent` appear as:

```
deploy-agent [BOT]  2:45 PM
┌─────────────────────────────────────┐
│ Completed | Deployment finished     │
│                                     │
│ All health checks passing.          │
│                                     │
│ [View Logs]                         │
└─────────────────────────────────────┘
```

Rather than:

```
Scion [BOT]  2:45 PM
┌─────────────────────────────────────┐
│ 🤖 deploy-agent                     │
│ Completed | Deployment finished     │
│ ...                                 │
└─────────────────────────────────────┘
```

When per-agent identity is active, the `CardHeader` can be simplified — the agent name is already visible as the message sender. The Block Kit renderer can optionally omit the header icon/title when `AgentID` is set, since it would be redundant.

---

## Threading

### Thread Model

Slack uses message timestamps (`ts`) as thread identifiers. Every message has a `ts`; a reply sets `thread_ts` to the parent message's `ts`.

| Concept | Google Chat | Slack |
|---------|------------|-------|
| Thread ID type | Resource name string | Message timestamp string |
| Thread reference | `thread.name` | `thread_ts` |
| New thread | Omit thread, or use unique `threadKey` | Omit `thread_ts` |
| Reply to thread | Set `thread.name` | Set `thread_ts` to parent's `ts` |
| Thread auto-creation | Every message is implicitly a thread root | Same |

### Implementation

The adapter normalizes Slack's `thread_ts` to the `ChatEvent.ThreadID` field:

```go
// For message events
event.ThreadID = slackEvent.ThreadTimeStamp
if event.ThreadID == "" {
    event.ThreadID = slackEvent.TimeStamp // top-level message is its own thread root
}
```

When sending replies, the `SendMessageRequest.ThreadID` is passed as `slackapi.MsgOptionTS()`:

```go
if req.ThreadID != "" {
    opts = append(opts, slackapi.MsgOptionTS(req.ThreadID))
}
```

The `/scion message --thread <thread-id>` command works the same way: the thread ID is a Slack message timestamp like `1712345678.123456`. This is included in notification cards so users can reference it when replying to agent conversations.

### Broadcast to Channel

Slack threads can optionally broadcast a reply to the main channel. This is useful for important notifications that shouldn't be buried in a thread:

```go
if req.ThreadID != "" && shouldBroadcast {
    opts = append(opts, slackapi.MsgOptionBroadcast())
}
```

This is a Slack-specific enhancement not available in Google Chat. The broadcast policy is **configurable per-channel** via a slash command:

```
/scion broadcast ERROR WAITING_FOR_INPUT    # broadcast these activity types
/scion broadcast all                         # broadcast all notifications
/scion broadcast none                        # never broadcast (default)
/scion broadcast                             # show current setting
```

The setting is stored in a new `space_settings` table in SQLite (see State Management below). When a notification arrives for a threaded conversation, the adapter checks the channel's broadcast policy against the notification's activity type to decide whether to add `MsgOptionBroadcast()`.

```go
type SpaceSettings struct {
    SpaceID            string
    Platform           string
    BroadcastActivities string // comma-separated: "ERROR,WAITING_FOR_INPUT", "all", or ""
}
```

The `broadcast` subcommand is added to the command router alongside `link`/`unlink`. Only channel admins or grove admins can change the broadcast policy for a space.

---

## User Identity & Mentions

### Email-Based Identity Mapping

The identity mapping flow is the same as Google Chat, but the email retrieval mechanism differs:

**Google Chat:** The user's email is included in every webhook payload as a Google-asserted identity. The `eventUserLookup` struct in `commands.go` returns it directly from the event.

**Slack:** The user's email is not in event payloads. The adapter must call `users.info` to retrieve it. The `GetUser()` method on the Messenger interface already supports this, but the identity mapper's `UserLookup` interface needs the adapter to provide the email:

```go
// slackUserLookup implements identity.UserLookup for Slack events.
type slackUserLookup struct {
    adapter *Adapter
}

func (sl *slackUserLookup) GetUser(ctx context.Context, userID string) (*identity.ChatUserInfo, error) {
    user, err := sl.adapter.GetUser(ctx, userID)
    if err != nil {
        return nil, err
    }
    return &identity.ChatUserInfo{
        PlatformID: user.PlatformID,
        Email:      user.Email,
    }, nil
}
```

**Caching:** To avoid hitting Slack's rate limits (Tier 4: ~100 req/min for `users.info`), the adapter caches user profile lookups in memory with a TTL:

```go
type userCache struct {
    mu    sync.RWMutex
    users map[string]*cachedUser
}

type cachedUser struct {
    user      *chatapp.ChatUser
    fetchedAt time.Time
}

const userCacheTTL = 15 * time.Minute
```

### @Mention Formatting

Slack @mentions use the format `<@U0ABC123>` (wrapping the user ID in angle brackets with an `@` prefix). The `NotificationRelay.getSubscriberMentions()` currently formats mentions as `<users/12345>` (Google Chat format). The relay needs platform-aware mention formatting.

**Approach:** The `NotificationRelay` already has access to the `Messenger` and the `SpaceLink.Platform` field. The mention formatting can be dispatched based on platform:

```go
func formatMention(platform, platformUserID string) string {
    switch platform {
    case "slack":
        return fmt.Sprintf("<@%s>", platformUserID)
    case "google_chat":
        return fmt.Sprintf("<users/%s>", platformUserID)
    default:
        return platformUserID
    }
}
```

This is a minor change to `notifications.go` — the only modification to shared code required for Slack support.

---

## App Home Tab

Slack's App Home provides a dedicated tab within the Scion bot's profile. When a user opens it, Slack sends an `app_home_opened` event. The adapter responds by publishing a view with the user's registration status, linked groves, and agent overview.

### Home Tab Content

```
┌─────────────────────────────────────────────────────────┐
│  Scion                                          [Refresh]│
├─────────────────────────────────────────────────────────┤
│                                                         │
│  *Your Profile*                                         │
│  Registration: Registered                               │
│  Hub Email: alice@example.com                           │
│  Hub: hub.example.com                                   │
│                                                         │
│  ─────────────────────────────────────────────────────  │
│                                                         │
│  *Linked Groves*                                        │
│                                                         │
│  #deploy-channel → production (3 agents)                │
│  #ml-team → ml-experiments (5 agents)                   │
│                                                         │
│  ─────────────────────────────────────────────────────  │
│                                                         │
│  *Your Subscriptions*                                   │
│                                                         │
│  deploy-agent (production) — ERROR, COMPLETED           │
│  trainer-agent (ml-experiments) — all activities         │
│                                                         │
│  ─────────────────────────────────────────────────────  │
│                                                         │
│  *Quick Actions*                                        │
│  [Register] [Link a Channel] [Help]                     │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Implementation

```go
// apphome.go

func (a *Adapter) handleAppHomeOpened(ctx context.Context, userID string) error {
    // Build the home view based on the user's state
    view := a.buildHomeView(ctx, userID)
    _, err := a.client.PublishViewContext(ctx, userID, view, "")
    return err
}

func (a *Adapter) buildHomeView(ctx context.Context, userID string) slackapi.HomeTabViewRequest {
    var blocks []slackapi.Block

    // Header
    blocks = append(blocks,
        slackapi.NewHeaderBlock(slackapi.NewTextBlockObject("plain_text", "Scion", false, false)),
        slackapi.NewDividerBlock(),
    )

    // User profile section
    // ... build based on identity mapper state

    // Linked groves section
    // ... build based on space links in state store

    // Subscriptions section
    // ... build based on agent subscriptions

    // Quick actions
    // ... buttons for common operations

    return slackapi.HomeTabViewRequest{
        Type:   "home",
        Blocks: slackapi.Blocks{BlockSet: blocks},
    }
}
```

The App Home tab is published via `views.publish` and updates reactively when state changes (space linked, user registered, subscription changed). After any state-changing command, the adapter re-publishes the home view for the acting user.

---

## Slash Command Handling

### Single Command, Subcommand Parsing

Like Google Chat, Slack registers a single `/scion` slash command. All subcommands are parsed from the text argument. The existing `CommandRouter.handleCommand()` already handles this parsing — the Slack adapter just needs to normalize the slash command payload to a `ChatEvent`:

```
/scion list          → ChatEvent{Type: EventCommand, Command: "scion", Args: "list"}
/scion status my-bot → ChatEvent{Type: EventCommand, Command: "scion", Args: "status my-bot"}
```

### Response Timing

Slack requires a response within 3 seconds. For commands that need Hub API calls (which may take longer), the adapter:

1. Immediately responds with `"response_type": "ephemeral", "text": "Processing..."` in the HTTP body
2. Processes the command asynchronously
3. Posts the result via `chat.postMessage` (visible to all) or `chat.postEphemeral` (visible only to the invoker)

Most commands should respond publicly (visible to all channel members). Some commands should be ephemeral:

| Command | Visibility | Reason |
|---------|-----------|--------|
| `help` | Ephemeral | Only the invoker needs to see it |
| `info` | Ephemeral | Contains user-specific registration info |
| `register` / `unregister` | Ephemeral | User-specific auth flow |
| `broadcast` | Ephemeral | Channel admin configuration |
| All others | Public | Team should see agent operations |

This is controlled by the adapter when translating `EventResponse` to Slack API calls.

---

## Configuration

### Extended SlackConfig

The existing `SlackConfig` struct is expanded:

```go
type SlackConfig struct {
    Enabled       bool   `yaml:"enabled"`
    BotToken      string `yaml:"bot_token"`       // xoxb-...
    AppToken      string `yaml:"app_token"`        // xapp-... (Socket Mode)
    SigningSecret string `yaml:"signing_secret"`   // HTTP mode verification
    ListenAddress string `yaml:"listen_address"`   // HTTP listen address
    SocketMode    bool   `yaml:"socket_mode"`      // Use Socket Mode
}
```

### Full Configuration Example

```yaml
# scion-chat-app.yaml

hub:
  endpoint: "https://hub.example.com"
  user: "chat-app@example.com"

plugin:
  listen_address: "localhost:9090"

platforms:
  google_chat:
    enabled: false

  slack:
    enabled: true
    bot_token: "${SLACK_BOT_TOKEN}"
    app_token: "${SLACK_APP_TOKEN}"        # Only needed for socket_mode
    signing_secret: "${SLACK_SIGNING_SECRET}"
    listen_address: ":8444"
    socket_mode: false

state:
  database: "/var/lib/scion-chat-app/state.db"

notifications:
  trigger_activities:
    - COMPLETED
    - WAITING_FOR_INPUT
    - ERROR
    - STALLED
    - LIMITS_EXCEEDED

logging:
  level: "info"
  format: "json"
```

---

## Changes to Shared Code

The Slack adapter is designed to require minimal changes to shared code. The following modifications are needed:

### 1. Platform-Aware Mention Formatting (notifications.go)

The `getSubscriberMentions()` and `buildMentions()` methods currently hardcode Google Chat mention format (`<users/ID>`). These need to dispatch based on `link.Platform`:

```go
func formatMention(platform, userID string) string {
    switch platform {
    case "slack":
        return fmt.Sprintf("<@%s>", userID)
    default:
        return fmt.Sprintf("<users/%s>", userID)
    }
}
```

### 2. User Email Lookup (commands.go)

The `eventUserLookup` struct returns `event.UserEmail` directly from the Google Chat event payload. For Slack, the email is not in the event. The `ChatEvent.UserEmail` field will be empty for Slack events.

The `ResolveOrAutoRegister()` call already accepts a `UserLookup` interface that can call `GetUser()` on the messenger. The Slack adapter provides a `slackUserLookup` that calls `users.info` to retrieve the email. No change to `commands.go` is needed — the lookup is injected by the adapter.

However, the `eventUserLookup` in `commands.go` needs to handle the case where `event.UserEmail` is empty (Slack) by falling back to the messenger's `GetUser()`:

```go
type eventUserLookup struct {
    event     *ChatEvent
    messenger Messenger
}

func (el *eventUserLookup) GetUser(ctx context.Context, userID string) (*identity.ChatUserInfo, error) {
    if el.event.UserEmail != "" {
        return &identity.ChatUserInfo{
            PlatformID: el.event.UserID,
            Email:      el.event.UserEmail,
        }, nil
    }
    // Fallback to messenger API (Slack path)
    user, err := el.messenger.GetUser(ctx, el.event.UserID)
    if err != nil {
        return nil, err
    }
    return &identity.ChatUserInfo{
        PlatformID: user.PlatformID,
        Email:      user.Email,
    }, nil
}
```

### 3. main.go Adapter Initialization

Add Slack adapter startup alongside Google Chat:

```go
if cfg.Platforms.Slack.Enabled {
    slackAdapter := slack.NewAdapter(slack.Config{
        BotToken:      cfg.Platforms.Slack.BotToken,
        AppToken:      cfg.Platforms.Slack.AppToken,
        SigningSecret: cfg.Platforms.Slack.SigningSecret,
        ListenAddress: cfg.Platforms.Slack.ListenAddress,
        SocketMode:    cfg.Platforms.Slack.SocketMode,
    }, router.HandleEvent, log.With("component", "slack"))

    router.SetMessenger(slackAdapter) // or use a multi-messenger dispatcher

    if cfg.Platforms.Slack.SocketMode {
        go slackAdapter.StartSocketMode()
    } else {
        go slackAdapter.Start(cfg.Platforms.Slack.ListenAddress)
    }
}
```

### 4. Multi-Platform Messenger Dispatch

When both Google Chat and Slack are enabled simultaneously, the `CommandRouter` currently holds a single `Messenger`. For multi-platform support, a dispatcher is needed:

```go
type MultiMessenger struct {
    adapters map[string]chatapp.Messenger // platform name → adapter
}

func (m *MultiMessenger) SendMessage(ctx context.Context, req chatapp.SendMessageRequest) (string, error) {
    // Route based on SpaceID format or lookup
    adapter := m.resolveAdapter(req.SpaceID)
    return adapter.SendMessage(ctx, req)
}
```

The `CommandRouter` already receives the platform from `ChatEvent.Platform`, so it can route responses to the correct adapter. The `NotificationRelay` also uses `SpaceLink.Platform` to determine which spaces to notify.

**Simpler alternative:** Since the `CommandRouter.HandleEvent()` receives events from a specific adapter and responds via the same adapter's messenger, the router can hold multiple messengers keyed by platform and select the right one based on `event.Platform`. This avoids a dispatcher abstraction:

```go
type CommandRouter struct {
    messengers map[string]Messenger // "google_chat" → gchat, "slack" → slack
    // ...
}

func (r *CommandRouter) messengerFor(platform string) Messenger {
    return r.messengers[platform]
}
```

### 5. State Schema Extension (state.go)

A new `space_settings` table stores per-channel configuration such as broadcast policy:

```sql
CREATE TABLE space_settings (
    space_id             TEXT NOT NULL,
    platform             TEXT NOT NULL,
    broadcast_activities TEXT NOT NULL DEFAULT '',  -- Comma-separated: "ERROR,WAITING_FOR_INPUT", "all", or ""
    PRIMARY KEY (space_id, platform)
);
```

The `Store` gains `GetSpaceSettings()`, `SetSpaceSettings()`, and `DeleteSpaceSettings()` methods. The settings are deleted automatically when a space is unlinked.

### 6. Command Router: `/scion broadcast` (commands.go)

A new `cmdBroadcast()` handler is added to the command router:

- With arguments: parses activity types (e.g., `ERROR WAITING_FOR_INPUT`) or keywords (`all`, `none`) and saves to `space_settings`
- Without arguments: displays the current broadcast policy for the space
- Requires the user to be a grove admin or the Slack channel manager

This command is platform-agnostic in the router but only has practical effect for Slack (Google Chat does not support thread broadcast). The router stores the setting regardless of platform; the adapter decides whether to act on it.

---

## Rate Limiting & API Constraints

Slack enforces rate limits on API calls. The adapter must respect these:

| API Method | Tier | Approx. Limit | Mitigation |
|-----------|------|---------------|------------|
| `chat.postMessage` | Tier 3 | ~50 req/min per channel | Queue outbound messages; batch notifications |
| `chat.update` | Tier 3 | ~50 req/min | Avoid rapid updates |
| `views.open` | Tier 4 | ~100 req/min | Unlikely to hit |
| `views.publish` | Special | ~1 req/sec per user | Debounce home tab updates |
| `users.info` | Tier 4 | ~100 req/min | Cache user profiles (15 min TTL) |

The `slack-go/slack` library handles `429 Too Many Requests` responses and retries automatically when configured. The adapter enables this:

```go
client := slackapi.New(botToken, slackapi.OptionHTTPClient(httpClient))
```

For notification fan-out (sending cards to multiple spaces for a grove), the adapter should serialize sends with a small delay to avoid hitting per-channel limits.

---

## Testing Strategy

### Unit Tests

- **Block Kit rendering:** Verify `renderBlocks()` produces correct block structures for all widget types. Compare JSON output against Slack's Block Kit Builder reference.
- **Modal rendering:** Verify `renderModal()` produces valid modal view requests.
- **Event normalization:** Verify all Slack event types normalize correctly to `ChatEvent`.
- **Signature verification:** Test `verifyRequest()` with valid and invalid signatures.
- **User cache:** Test cache hit/miss/expiry behavior.

### Integration Tests

- **Round-trip:** Simulate a slash command → command router → response → Block Kit render cycle.
- **Modal flow:** Simulate interaction → `views.open` → submission → `EventDialogSubmit` → response.
- **Socket Mode:** Verify event handling through the Socket Mode client.

### Manual Testing with Slack

Slack provides a sandbox workspace for app development. Use Socket Mode for local development (no public URL needed). The `slack-go/slack` library includes a test server for unit testing without hitting Slack's API.

---

## Implementation Plan

### Phase 3a: Core Slack Adapter

- [ ] Add `github.com/slack-go/slack` dependency to `extras/scion-chat-app/go.mod`
- [ ] Implement `internal/slack/adapter.go` — `Adapter` struct, `Start()`, `Stop()`, HTTP endpoints
- [ ] Implement `internal/slack/events.go` — event normalization for slash commands, app_mention, interactions
- [ ] Implement `internal/slack/verify.go` — request signature verification
- [ ] Implement `internal/slack/blocks.go` — `Card` → Block Kit rendering for all widget types
- [ ] Implement `internal/slack/modals.go` — `Dialog` → Slack Modal rendering + value extraction
- [ ] Expand `SlackConfig` struct in `config.go` (`app_token`, `socket_mode`)
- [ ] Wire Slack adapter in `main.go` alongside Google Chat
- [ ] Update `eventUserLookup` in `commands.go` to fall back to `GetUser()` when email is absent
- [ ] Add platform-aware mention formatting to `notifications.go`
- [ ] Unit tests for block rendering, modal rendering, and event normalization

### Phase 3b: Dynamic Identity & Threading

- [ ] Implement per-agent username/avatar via `chat:write.customize`
- [ ] Implement `IconProvider` abstraction with `robohashProvider` default
- [ ] Implement Socket Mode support as an alternative to HTTP
- [ ] Thread ID (`thread_ts`) round-trip through `SendMessageRequest` → `ChatEvent`
- [ ] Add `space_settings` table and `/scion broadcast` command for per-channel broadcast policy
- [ ] Broadcast reply support based on per-channel activity type configuration

### Phase 3c: App Home & Polish

- [ ] Implement `internal/slack/apphome.go` — App Home tab rendering
- [ ] Publish home view on `app_home_opened` events
- [ ] Re-publish home view after state-changing commands
- [ ] User profile cache with TTL for `users.info` calls
- [ ] Multi-platform messenger dispatch (for simultaneous Google Chat + Slack)
- [ ] Ephemeral vs. public response routing for slash commands
- [ ] End-to-end testing in Slack sandbox workspace

---

## Resolved Decisions

1. **SDK choice:** `github.com/slack-go/slack` — the most widely used Go Slack client, actively maintained, covers Web API, Events API, Socket Mode, and Block Kit.

2. **Socket Mode support:** Yes. Socket Mode is supported as a deployment option for environments without a public HTTP endpoint. It uses the same event normalization as HTTP mode.

3. **Synchronous response handling:** The adapter acknowledges all events within 3 seconds (Slack's requirement) and sends rich responses asynchronously via the Web API. This differs from Google Chat's synchronous HTTP response pattern but is transparent to the `CommandRouter` — it returns `EventResponse` either way.

4. **Per-agent identity:** Enabled by default when the bot has `chat:write.customize` scope. Falls back to card-header-based identity (like Google Chat) if the scope is missing.

5. **User email retrieval:** Calls `users.info` API with in-memory caching (15 min TTL). This replaces Google Chat's event-embedded email. The `eventUserLookup` in shared code falls back to `Messenger.GetUser()` when `event.UserEmail` is empty.

6. **App Home tab:** Included as a dashboard surface. Published on `app_home_opened` and refreshed after state changes. Not a critical-path feature — can be deferred if Phase 3 scope needs trimming.

7. **Multi-platform dispatch:** When both adapters are enabled, the `CommandRouter` holds messengers keyed by platform name. Events route to the originating adapter. No abstract dispatcher layer needed.

8. **Ephemeral responses:** `help`, `info`, `register`, and `unregister` responses are sent as ephemeral messages (visible only to the invoker). All other command responses are public.

9. **Agent icon generation:** Use [RoboHash](https://robohash.org/) for deterministic robot-themed agent avatars, seeded by agent slug. The implementation is behind an `IconProvider` abstraction so the source can be swapped later without changing the adapter.

10. **Broadcast reply policy:** Configurable per-channel via `/scion broadcast <activity-types>`. Channel owners/grove admins can set which notification activity types broadcast from threads to the main channel. Defaults to none (no broadcast). Stored in a `space_settings` table in SQLite.

11. **Channel type restrictions:** Public channels and DMs only for MVP. Private channels (`groups:read`, `mpim:read` scopes) are deferred to a future phase. DMs to the bot work via `message.im` and are sufficient for user-specific interactions like registration.

## Open Questions

_None at this time._
