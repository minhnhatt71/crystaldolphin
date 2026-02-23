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

// DingTalkChannel connects to DingTalk via Stream Mode (WebSocket).
type DingTalkChannel struct {
	Base
	cfg        *channel.DingTalkConfig
	httpClient *http.Client
	token      string
	tokenMu    sync.Mutex
	tokenExp   time.Time
}

func NewDingTalkChannel(cfg *channel.DingTalkConfig, b *bus.MessageBus) *DingTalkChannel {
	return &DingTalkChannel{
		Base:       NewBase("dingtalk", b, cfg.AllowFrom),
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (d *DingTalkChannel) Name() string { return "dingtalk" }

func (d *DingTalkChannel) Start(ctx context.Context) error {
	if d.cfg.ClientID == "" || d.cfg.ClientSecret == "" {
		slog.Warn("dingtalk: clientId or clientSecret not configured")
		<-ctx.Done()
		return ctx.Err()
	}
	for {
		if err := d.connectOnce(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (d *DingTalkChannel) connectOnce(ctx context.Context) error {
	endpoint, ticket, err := d.getStreamEndpoint(ctx)
	if err != nil {
		slog.Warn("dingtalk: get stream endpoint failed", "err", err)
		return err
	}

	wsURL := endpoint + "?ticket=" + ticket
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	slog.Info("dingtalk: stream connected")

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var frame map[string]any
		if err := json.Unmarshal(raw, &frame); err != nil {
			continue
		}
		// Acknowledge the frame.
		msgID, _ := frame["messageId"].(string)
		ack, _ := json.Marshal(map[string]any{
			"code":      200,
			"headers":   map[string]string{"messageId": msgID},
			"message":   "OK",
			"requestId": msgID,
		})
		_ = conn.WriteMessage(websocket.TextMessage, ack)

		go d.handleFrame(frame)
	}
}

func (d *DingTalkChannel) getStreamEndpoint(ctx context.Context) (endpoint, ticket string, err error) {
	token, err := d.getAccessToken(ctx)
	if err != nil {
		return "", "", err
	}

	body := map[string]any{
		"clientId":     d.cfg.ClientID,
		"clientSecret": d.cfg.ClientSecret,
		"subscriptions": []map[string]string{
			{"type": "EVENT", "topic": "*"},
			{"type": "CALLBACK", "topic": "/v1.0/im/bot/messages/get"},
		},
		"ua":      "crystaldolphin",
		"localIp": "127.0.0.1",
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/gateway/connections/open", bytes.NewReader(data))
	req.Header.Set("x-acs-dingtalk-access-token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result struct {
		Endpoint string `json:"endpoint"`
		Ticket   string `json:"ticket"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return "", "", err
	}
	if result.Endpoint == "" {
		return "", "", fmt.Errorf("dingtalk: no endpoint returned: %s", string(b))
	}
	return result.Endpoint, result.Ticket, nil
}

func (d *DingTalkChannel) getAccessToken(ctx context.Context) (string, error) {
	d.tokenMu.Lock()
	defer d.tokenMu.Unlock()
	if d.token != "" && time.Now().Before(d.tokenExp) {
		return d.token, nil
	}
	body := map[string]string{"clientId": d.cfg.ClientID, "clientSecret": d.cfg.ClientSecret,
		"grantType": "client_credentials"}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/oauth2/accessToken", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &result)
	if result.AccessToken == "" {
		return "", fmt.Errorf("dingtalk: get token failed")
	}
	d.token = result.AccessToken
	d.tokenExp = time.Now().Add(time.Duration(result.ExpireIn-60) * time.Second)
	return d.token, nil
}

func (d *DingTalkChannel) handleFrame(frame map[string]any) {
	headers, _ := frame["headers"].(map[string]any)
	topic, _ := headers["topic"].(string)

	if topic != "/v1.0/im/bot/messages/get" {
		return
	}

	var data struct {
		SenderID       string `json:"senderId"`
		ConversationID string `json:"conversationId"`
		Text           struct {
			Content string `json:"content"`
		} `json:"text"`
		MessageType string `json:"msgtype"`
	}
	rawData, _ := json.Marshal(frame["data"])
	if err := json.Unmarshal(rawData, &data); err != nil {
		return
	}

	if data.MessageType != "text" {
		return
	}

	content := strings.TrimSpace(data.Text.Content)
	if content == "" {
		return
	}

	d.HandleMessage(data.SenderID, data.ConversationID, content, nil, map[string]any{
		"topic": topic,
	})
}

func (d *DingTalkChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	token, err := d.getAccessToken(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{
		"robotCode": d.cfg.ClientID,
		"userIds":   []string{msg.ChatID},
		"msgKey":    "sampleText",
		"msgParam":  `{"content":"` + escapeDingTalk(msg.Content) + `"}`,
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend", bytes.NewReader(data))
	req.Header.Set("x-acs-dingtalk-access-token", token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func escapeDingTalk(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
