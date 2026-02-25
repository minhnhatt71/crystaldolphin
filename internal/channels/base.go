// Package channels provides chat-platform channel implementations.
package channels

import (
	"log/slog"
	"strings"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// Channel is the interface every platform must implement.
// The canonical definition lives in internal/interfaces; this alias keeps
// existing code compiling without changes.
type Channel = schema.Channel

// Base holds common state and helper methods shared by all channels.
type Base struct {
	channelName string
	b           *bus.MessageBus
	allowFrom   []string // empty = allow all
}

// NewBase creates a Base with the given channel name, bus, and allowlist.
func NewBase(name string, b *bus.MessageBus, allowFrom []string) Base {
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
	senderID, chatID, content string,
	media []string,
	metadata map[string]any,
) {
	if !b.IsAllowed(senderID) {
		slog.Warn("access denied", "channel", b.channelName, "sender", senderID)
		return
	}
	b.b.Inbound <- bus.InboundMessage{
		Channel:   b.channelName,
		SenderID:  senderID,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Now(),
		Media:     media,
		Metadata:  metadata,
	}
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
