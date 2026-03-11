package bus

import (
	"strings"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/modeling/channel"
)

// InboundMessage is a message received from a chat platform and delivered to the agent.
// It is a pure value type — no I/O, no dependencies.
type InboundMessage struct {
	channelName channel.ChannelName
	senderId    string
	chatID      string
	routingKey  string // empty → derived from channelName:chatID
	content     string
	timestamp   time.Time
	media       []string
	metadata    map[string]any
}

// NewInboundMessage constructs an InboundMessage with Timestamp set to now.
// Pass routingKey="" to derive it automatically as "channelName:chatID".
func NewInboundMessage(ch channel.ChannelName, senderId, chatID, content, routingKey string) InboundMessage {
	key := routingKey
	if key == "" {
		key = inboundRoutingKey(ch, chatID)
	}

	return InboundMessage{
		channelName: ch,
		senderId:    senderId,
		chatID:      chatID,
		content:     content,
		routingKey:  key,
		timestamp:   time.Now(),
	}
}

// WithMedia returns a copy with media set.
func (m InboundMessage) WithMedia(media []string) InboundMessage {
	m.media = media
	return m
}

// WithMetadata returns a copy with metadata set.
func (m InboundMessage) WithMetadata(metadata map[string]any) InboundMessage {
	m.metadata = metadata
	return m
}

func (m InboundMessage) ChannelName() channel.ChannelName { return m.channelName }
func (m InboundMessage) SenderID() string                 { return m.senderId }
func (m InboundMessage) ChatID() string                   { return m.chatID }
func (m InboundMessage) Content() string                  { return m.content }
func (m InboundMessage) RoutingKey() string               { return m.routingKey }
func (m InboundMessage) Timestamp() time.Time             { return m.timestamp }
func (m InboundMessage) Media() []string                  { return m.media }
func (m InboundMessage) Metadata() map[string]any         { return m.metadata }

// inboundRoutingKey derives the session-lookup key from channel and chat ID.
// Mirrors nanobot's InboundMessage.session_key: "channel:chat_id".
func inboundRoutingKey(ch channel.ChannelName, chatID string) string {
	if chatID == "" {
		return string(ch)
	}
	return string(ch) + ":" + chatID
}

// ParseRoutingKey splits a routing key back into channel name and chat ID.
func ParseRoutingKey(key string) (channel.ChannelName, string) {
	ch, chatID, _ := strings.Cut(key, ":")
	return channel.ChannelName(ch), chatID
}
