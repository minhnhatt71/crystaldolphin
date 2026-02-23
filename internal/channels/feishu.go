package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config/channel"
)

// FeishuChannel connects to Feishu/Lark via WebSocket long connection.
// It uses the Feishu Open Platform event subscription (WebSocket mode).
type FeishuChannel struct {
	Base
	cfg        *channel.FeishuConfig
	httpClient *http.Client
	token      string
	tokenMu    sync.Mutex
	tokenExp   time.Time
}

func NewFeishuChannel(cfg *channel.FeishuConfig, b *bus.MessageBus) *FeishuChannel {
	return &FeishuChannel{
		Base:       NewBase("feishu", b, cfg.AllowFrom),
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (f *FeishuChannel) Name() string { return "feishu" }

func (f *FeishuChannel) Start(ctx context.Context) error {
	if f.cfg.AppID == "" || f.cfg.AppSecret == "" {
		slog.Warn("feishu: appId or appSecret not configured")
		<-ctx.Done()
		return ctx.Err()
	}
	for {
		if err := f.connectOnce(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (f *FeishuChannel) connectOnce(ctx context.Context) error {
	// Get endpoint from Feishu WebSocket API.
	wsURL, err := f.getWebSocketURL(ctx)
	if err != nil {
		slog.Warn("feishu: get ws url failed", "err", err)
		return err
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	slog.Info("feishu: connected")

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var frame struct {
			Type    string            `json:"type"`
			Headers map[string]string `json:"headers"`
			Data    json.RawMessage   `json:"data"`
		}
		if err := json.Unmarshal(raw, &frame); err != nil {
			continue
		}

		// Send pong for ping frames.
		if frame.Type == "ping" {
			pong := map[string]any{"type": "pong"}
			pongData, _ := json.Marshal(pong)
			_ = conn.WriteMessage(websocket.TextMessage, pongData)
			continue
		}

		if frame.Type != "event" {
			continue
		}

		go f.handleEvent(frame.Data)
	}
}

func (f *FeishuChannel) getWebSocketURL(ctx context.Context) (string, error) {
	token, err := f.getAccessToken(ctx)
	if err != nil {
		return "", err
	}
	body := map[string]any{"app_id": f.cfg.AppID}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://open.feishu.cn/open-apis/event/v1/ws/endpoint", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Code int `json:"code"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu: get ws url code=%d msg=%s", result.Code, result.Msg)
	}
	return result.Data.URL, nil
}

func (f *FeishuChannel) getAccessToken(ctx context.Context) (string, error) {
	f.tokenMu.Lock()
	defer f.tokenMu.Unlock()
	if f.token != "" && time.Now().Before(f.tokenExp) {
		return f.token, nil
	}
	body := map[string]string{"app_id": f.cfg.AppID, "app_secret": f.cfg.AppSecret}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &result)
	if result.TenantAccessToken == "" {
		return "", fmt.Errorf("feishu: get token failed")
	}
	f.token = result.TenantAccessToken
	f.tokenExp = time.Now().Add(time.Duration(result.Expire-60) * time.Second)
	return f.token, nil
}

func (f *FeishuChannel) handleEvent(data json.RawMessage) {
	var event struct {
		Schema string `json:"schema"`
		Header struct {
			EventType string `json:"event_type"`
		} `json:"header"`
		Event struct {
			Message struct {
				MessageID   string `json:"message_id"`
				ChatID      string `json:"chat_id"`
				ChatType    string `json:"chat_type"`
				Content     string `json:"content"`
				MessageType string `json:"message_type"`
			} `json:"message"`
			Sender struct {
				SenderID struct {
					OpenID string `json:"open_id"`
				} `json:"sender_id"`
				SenderType string `json:"sender_type"`
			} `json:"sender"`
		} `json:"event"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}

	// Only handle im.message.receive_v1 events.
	if event.Header.EventType != "im.message.receive_v1" {
		return
	}
	if event.Event.Sender.SenderType != "user" {
		return
	}

	senderID := event.Event.Sender.SenderID.OpenID
	chatID := event.Event.Message.ChatID
	msgType := event.Event.Message.MessageType
	rawContent := event.Event.Message.Content

	// Extract text from JSON content.
	text := extractFeishuText(msgType, rawContent)
	if text == "" {
		return
	}

	f.HandleMessage(senderID, chatID, text, nil, map[string]any{
		"message_id": event.Event.Message.MessageID,
		"chat_type":  event.Event.Message.ChatType,
		"msg_type":   msgType,
	})
}

func extractFeishuText(msgType, rawContent string) string {
	var content map[string]any
	if err := json.Unmarshal([]byte(rawContent), &content); err != nil {
		return rawContent
	}
	switch msgType {
	case "text":
		if t, ok := content["text"].(string); ok {
			return strings.TrimSpace(t)
		}
	case "post":
		// Rich text — extract all text segments.
		var parts []string
		extractPostText(content, &parts)
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	return rawContent
}

func extractPostText(v any, parts *[]string) {
	switch val := v.(type) {
	case map[string]any:
		if tag, _ := val["tag"].(string); tag == "text" {
			if t, ok := val["text"].(string); ok {
				*parts = append(*parts, t)
			}
		}
		for _, child := range val {
			extractPostText(child, parts)
		}
	case []any:
		for _, item := range val {
			extractPostText(item, parts)
		}
	}
}

func (f *FeishuChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	token, err := f.getAccessToken(ctx)
	if err != nil {
		return err
	}

	// Determine receive_id_type based on chat_id prefix.
	idType := "chat_id"
	if strings.HasPrefix(msg.ChatID, "ou_") {
		idType = "open_id"
	}

	body := map[string]any{
		"receive_id": msg.ChatID,
		"msg_type":   "text",
		"content":    `{"text":"` + escapeFeishuText(msg.Content) + `"}`,
	}
	data, _ := json.Marshal(body)

	url := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=" + idType
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func escapeFeishuText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
