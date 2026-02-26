package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config/channel"
)

// WhatsAppChannel connects to the Node.js Baileys bridge via WebSocket.
type WhatsAppChannel struct {
	Base
	cfg       *channel.WhatsAppConfig
	conn      *websocket.Conn
	connected bool
}

func NewWhatsAppChannel(cfg *channel.WhatsAppConfig, b bus.Bus) *WhatsAppChannel {
	return &WhatsAppChannel{
		Base: NewBase("whatsapp", b, cfg.AllowFrom),
		cfg:  cfg,
	}
}

func (w *WhatsAppChannel) Name() string { return "whatsapp" }

func (w *WhatsAppChannel) Start(ctx context.Context) error {
	bridgeURL := w.cfg.BridgeURL
	if bridgeURL == "" {
		bridgeURL = "ws://localhost:3001"
	}
	slog.Info("whatsapp: connecting to bridge", "url", bridgeURL)

	for {
		if err := w.connectOnce(ctx, bridgeURL); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("whatsapp: connection lost, reconnecting in 5s", "err", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (w *WhatsAppChannel) connectOnce(ctx context.Context, url string) error {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return err
	}
	w.conn = conn
	w.connected = true
	defer func() { conn.Close(); w.conn = nil; w.connected = false }()

	slog.Info("whatsapp: connected to bridge")

	if w.cfg.BridgeToken != "" {
		auth, _ := json.Marshal(map[string]string{"type": "auth", "token": w.cfg.BridgeToken})
		_ = conn.WriteMessage(websocket.TextMessage, auth)
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		go w.handleBridgeMessage(raw)
	}
}

func (w *WhatsAppChannel) handleBridgeMessage(raw []byte) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	msgType, _ := data["type"].(string)
	switch msgType {
	case "message":
		pn, _ := data["pn"].(string)
		sender, _ := data["sender"].(string)
		content, _ := data["content"].(string)

		userID := pn
		if userID == "" {
			userID = sender
		}
		senderID := userID
		if i := indexByte(userID, '@'); i >= 0 {
			senderID = userID[:i]
		}

		if content == "[Voice Message]" {
			content = "[Voice Message: Transcription not available for WhatsApp yet]"
		}

		chatID := sender
		if chatID == "" {
			chatID = userID
		}

		w.HandleMessage(senderID, chatID, content, nil, map[string]any{
			"message_id": data["id"],
			"timestamp":  data["timestamp"],
			"is_group":   data["isGroup"],
		})
	case "status":
		status, _ := data["status"].(string)
		slog.Info("whatsapp: status", "status", status)
		w.connected = status == "connected"
	case "qr":
		slog.Info("whatsapp: scan QR code in the bridge terminal")
	case "error":
		slog.Error("whatsapp: bridge error", "error", data["error"])
	}
}

func (w *WhatsAppChannel) Send(_ context.Context, msg bus.OutboundMessage) error {
	if w.conn == nil || !w.connected {
		return fmt.Errorf("whatsapp: bridge not connected")
	}
	payload, _ := json.Marshal(map[string]string{
		"type": "send",
		"to":   msg.ChatId(),
		"text": msg.Content(),
	})
	return w.conn.WriteMessage(websocket.TextMessage, payload)
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
