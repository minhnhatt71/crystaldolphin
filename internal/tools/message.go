package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
)

// MessageTool sends a message to the user on a chat channel.
// It holds per-turn context (channel, chat_id) set by the agent loop before each turn.
type MessageTool struct {
	bus            *bus.MessageBus
	defaultChannel string
	defaultChatID  string
	defaultMsgID   string
	sentInTurn     bool
}

// NewMessageTool creates a MessageTool backed by a MessageBus.
func NewMessageTool(b *bus.MessageBus) *MessageTool {
	return &MessageTool{bus: b}
}

// SetContext updates the default channel/chatID/msgID for the current turn.
// Called by the agent loop before each LLM iteration.
func (t *MessageTool) SetContext(channel, chatID, msgID string) {
	t.defaultChannel = channel
	t.defaultChatID = chatID
	t.defaultMsgID = msgID
}

// StartTurn resets the per-turn sent flag.
func (t *MessageTool) StartTurn() { t.sentInTurn = false }

// WasSentInTurn reports whether a message was sent during the current turn.
func (t *MessageTool) WasSentInTurn() bool { return t.sentInTurn }

func (t *MessageTool) Name() string        { return "message" }
func (t *MessageTool) Description() string { return "Send a message to the user. Use this when you want to communicate something." }
func (t *MessageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"content": {
				"type": "string",
				"description": "The message content to send"
			},
			"channel": {
				"type": "string",
				"description": "Optional: target channel (telegram, discord, etc.)"
			},
			"chat_id": {
				"type": "string",
				"description": "Optional: target chat/user ID"
			},
			"media": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Optional: list of file paths to attach (images, audio, documents)"
			}
		},
		"required": ["content"]
	}`)
}

func (t *MessageTool) Execute(_ context.Context, params map[string]any) (string, error) {
	content, _ := params["content"].(string)
	if content == "" {
		return "Error: content is required", nil
	}

	channel := t.defaultChannel
	if ch, ok := params["channel"].(string); ok && ch != "" {
		channel = ch
	}
	chatID := t.defaultChatID
	if cid, ok := params["chat_id"].(string); ok && cid != "" {
		chatID = cid
	}
	msgID := t.defaultMsgID
	if mid, ok := params["message_id"].(string); ok && mid != "" {
		msgID = mid
	}

	if channel == "" || chatID == "" {
		return "Error: No target channel/chat specified", nil
	}

	var media []string
	if m, ok := params["media"].([]any); ok {
		for _, item := range m {
			if s, ok := item.(string); ok {
				media = append(media, s)
			}
		}
	}

	metadata := map[string]any{}
	if msgID != "" {
		metadata["message_id"] = msgID
	}

	t.bus.Outbound <- bus.OutboundMessage{
		Channel:  channel,
		ChatID:   chatID,
		Content:  content,
		Media:    media,
		Metadata: metadata,
	}
	t.sentInTurn = true

	info := ""
	if len(media) > 0 {
		info = fmt.Sprintf(" with %d attachments", len(media))
	}
	return fmt.Sprintf("Message sent to %s:%s%s", channel, chatID, info), nil
}
