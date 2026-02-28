// Package channels provides chat-platform channel implementations.
package channels

import (
	"log/slog"
	"strings"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
)

// Base holds common state and helper methods shared by all channels.
type Base struct {
	channelName bus.ChannelType
	b           bus.Bus
	allowFrom   []string // empty = allow all
}

// NewBase creates a Base with the given channel name, bus, and allowlist.
func NewBase(name bus.ChannelType, b bus.Bus, allowFrom []string) Base {
	return Base{channelName: name, b: b, allowFrom: allowFrom}
}

// IsAllowed checks whether senderID is on the allowlist.
// senderID may be "id|username" (Telegram) or a plain string.
func (b *Base) IsAllowed(senderID string) bool {
	if len(b.allowFrom) == 0 {
		return true
	}
	s := senderID
	for _, allowed := range b.allowFrom {
		if allowed == s {
			return true
		}
	}
	// Handle "id|username" format used by Telegram.
	if strings.Contains(senderID, "|") {
		for _, part := range strings.Split(senderID, "|") {
			if part == "" {
				continue
			}
			for _, allowed := range b.allowFrom {
				if allowed == part {
					return true
				}
			}
		}
	}
	return false
}

// HandleMessage verifies the sender is allowed, then pushes an InboundMessage to the bus.
func (b *Base) HandleMessage(
	senderId, chatId, content string,
	media []string,
	metadata map[string]any,
) {
	if !b.IsAllowed(senderId) {
		slog.Warn("access denied", "channel", b.channelName, "sender", senderId)
		return
	}

	msg := bus.NewInboundMessage(b.channelName, senderId, chatId, content, "")
	msg.SetMedia(media)
	msg.SetMetadata(metadata)
	b.b.PublishInbound(msg)
}

// splitMessage splits content into chunks that fit within maxLen,
// preferring newline breaks, then space breaks, then hard cut.
// Mirrors Python's _split_message in telegram.py / discord.py.
func splitMessage(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}
	var chunks []string
	for len(content) > 0 {
		if len(content) <= maxLen {
			chunks = append(chunks, content)
			break
		}
		cut := content[:maxLen]
		pos := strings.LastIndex(cut, "\n")
		if pos <= 0 {
			pos = strings.LastIndex(cut, " ")
		}
		if pos <= 0 {
			pos = maxLen
		}
		chunks = append(chunks, content[:pos])
		content = strings.TrimLeft(content[pos:], " \t")
	}
	return chunks
}
