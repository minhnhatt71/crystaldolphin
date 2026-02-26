package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config/channel"
)

const (
	discordAPI       = "https://discord.com/api/v10"
	discordMaxMsgLen = 2000
	discordMaxFileB  = 20 * 1024 * 1024 // 20 MB
)

// DiscordChannel connects to the Discord Gateway WebSocket.
type DiscordChannel struct {
	Base
	cfg        *channel.DiscordConfig
	httpClient *http.Client
	conn       *websocket.Conn
	seq        *int
}

func NewDiscordChannel(cfg *channel.DiscordConfig, b bus.Bus) *DiscordChannel {
	return &DiscordChannel{
		Base:       NewBase("discord", b, cfg.AllowFrom),
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DiscordChannel) Name() string { return "discord" }

func (d *DiscordChannel) Start(ctx context.Context) error {
	if d.cfg.Token == "" {
		return fmt.Errorf("discord: token not configured")
	}
	for {
		if err := d.connect(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (d *DiscordChannel) connect(ctx context.Context) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, d.cfg.GatewayURL, nil)
	if err != nil {
		return err
	}
	d.conn = conn
	defer func() { conn.Close(); d.conn = nil }()
	slog.Info("discord: gateway connected")
	return d.gatewayLoop(ctx, conn)
}

func (d *DiscordChannel) gatewayLoop(ctx context.Context, conn *websocket.Conn) error {
	heartbeatStop := make(chan struct{})
	defer close(heartbeatStop)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var payload struct {
			Op int             `json:"op"`
			S  *int            `json:"s"`
			T  string          `json:"t"`
			D  json.RawMessage `json:"d"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		if payload.S != nil {
			d.seq = payload.S
		}

		switch payload.Op {
		case 10: // HELLO
			var hello struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			_ = json.Unmarshal(payload.D, &hello)
			interval := time.Duration(hello.HeartbeatInterval) * time.Millisecond
			go d.heartbeatLoop(ctx, conn, interval, heartbeatStop)
			if err := d.identify(conn); err != nil {
				return err
			}
		case 0: // DISPATCH
			if payload.T == "MESSAGE_CREATE" {
				var msg map[string]any
				if err := json.Unmarshal(payload.D, &msg); err == nil {
					go d.handleMessageCreate(ctx, msg)
				}
			}
		case 7, 9: // RECONNECT / INVALID_SESSION
			return fmt.Errorf("discord: gateway requested reconnect (op=%d)", payload.Op)
		}
	}
}

func (d *DiscordChannel) heartbeatLoop(ctx context.Context, conn *websocket.Conn, interval time.Duration, stop <-chan struct{}) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			payload := map[string]any{"op": 1, "d": d.seq}
			data, _ := json.Marshal(payload)
			_ = conn.WriteMessage(websocket.TextMessage, data)
		case <-stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (d *DiscordChannel) identify(conn *websocket.Conn) error {
	payload := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   d.cfg.Token,
			"intents": d.cfg.Intents,
			"properties": map[string]any{
				"os": "crystaldolphin", "browser": "crystaldolphin", "device": "crystaldolphin",
			},
		},
	}
	data, _ := json.Marshal(payload)
	return conn.WriteMessage(websocket.TextMessage, data)
}

func (d *DiscordChannel) handleMessageCreate(ctx context.Context, payload map[string]any) {
	author, _ := payload["author"].(map[string]any)
	if bot, _ := author["bot"].(bool); bot {
		return
	}
	senderID, _ := author["id"].(string)
	channelID, _ := payload["channel_id"].(string)
	if senderID == "" || channelID == "" {
		return
	}

	content, _ := payload["content"].(string)
	var parts []string
	if content != "" {
		parts = append(parts, content)
	}

	var mediaPaths []string
	if attachments, ok := payload["attachments"].([]any); ok {
		home, _ := os.UserHomeDir()
		mediaDir := filepath.Join(home, ".nanobot", "media")
		_ = os.MkdirAll(mediaDir, 0o755)

		for _, att := range attachments {
			a, ok := att.(map[string]any)
			if !ok {
				continue
			}
			url, _ := a["url"].(string)
			filename, _ := a["filename"].(string)
			if url == "" {
				continue
			}
			fileID, _ := a["id"].(string)
			dest := filepath.Join(mediaDir, fileID+"_"+safeFilename(filename))
			if err := downloadToFile(url, dest); err != nil {
				parts = append(parts, "[attachment: "+filename+" - download failed]")
				continue
			}
			mediaPaths = append(mediaPaths, dest)
			parts = append(parts, "[attachment: "+dest+"]")
		}
	}

	text := joinNonEmpty(parts, "\n")
	if text == "" {
		text = "[empty message]"
	}

	// Typing indicator.
	typingCtx, cancelTyping := context.WithCancel(ctx)
	defer cancelTyping()
	go d.sendTypingLoop(typingCtx, channelID)

	replyTo := ""
	if ref, ok := payload["referenced_message"].(map[string]any); ok {
		if rid, ok := ref["id"].(string); ok {
			replyTo = rid
		}
	}

	d.HandleMessage(senderID, channelID, text, mediaPaths, map[string]any{
		"message_id": payload["id"],
		"guild_id":   payload["guild_id"],
		"reply_to":   replyTo,
	})
}

func (d *DiscordChannel) sendTypingLoop(ctx context.Context, channelID string) {
	url := discordAPI + "/channels/" + channelID + "/typing"
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		req.Header.Set("Authorization", "Bot "+d.cfg.Token)
		resp, err := d.httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		select {
		case <-time.After(8 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

func (d *DiscordChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	url := discordAPI + "/channels/" + msg.ChatID() + "/messages"
	chunks := splitMessage(msg.Content(), discordMaxMsgLen)
	if len(chunks) == 0 {
		return nil
	}
	for i, chunk := range chunks {
		payload := map[string]any{"content": chunk}
		if i == 0 && msg.ReplyTo() != "" {
			payload["message_reference"] = map[string]any{"message_id": msg.ReplyTo()}
			payload["allowed_mentions"] = map[string]any{"replied_user": false}
		}
		if err := d.postJSON(ctx, url, payload); err != nil {
			slog.Error("discord: send failed", "err", err)
		}
	}
	return nil
}

func (d *DiscordChannel) postJSON(ctx context.Context, url string, payload any) error {
	data, _ := json.Marshal(payload)
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bot "+d.cfg.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := d.httpClient.Do(req)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 429 {
			var rate struct {
				RetryAfter float64 `json:"retry_after"`
			}
			_ = json.Unmarshal(body, &rate)
			d := time.Duration(rate.RetryAfter*1000) * time.Millisecond
			if d <= 0 {
				d = time.Second
			}
			time.Sleep(d)
			continue
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("discord: HTTP %d: %s", resp.StatusCode, string(body))
		}
		return nil
	}
	return fmt.Errorf("discord: max retries exceeded")
}

// downloadToFile fetches a URL and saves it to dest.
func downloadToFile(url, dest string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

func safeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func joinNonEmpty(parts []string, sep string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
