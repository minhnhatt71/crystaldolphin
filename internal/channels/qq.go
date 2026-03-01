package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config/channel"
)

// QQChannel connects to the QQ bot gateway WebSocket.
// Implements C2C (private) message handling, mirroring Python qq.py.
type QQChannel struct {
	Base
	cfg        *channel.QQConfig
	httpClient *http.Client
	token      string
	tokenMu    sync.Mutex
	tokenExp   time.Time
	// Dedup sliding window (1000 IDs).
	seenMu    sync.Mutex
	seen      map[string]bool
	seenQueue []string
}

func NewQQChannel(cfg *channel.QQConfig, b *bus.AgentBus) *QQChannel {
	return &QQChannel{
		Base:       NewBase("qq", b, cfg.AllowFrom),
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		seen:       make(map[string]bool),
	}
}

func (q *QQChannel) Name() string { return "qq" }

func (q *QQChannel) Start(ctx context.Context) error {
	if q.cfg.AppID == "" || q.cfg.Secret == "" {
		slog.Warn("qq: appId or secret not configured")
		<-ctx.Done()
		return ctx.Err()
	}
	for {
		if err := q.connectOnce(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (q *QQChannel) connectOnce(ctx context.Context) error {
	token, err := q.getAccessToken(ctx)
	if err != nil {
		slog.Warn("qq: get token failed", "err", err)
		return err
	}

	// Get WebSocket gateway URL.
	wsURL, err := q.getGatewayURL(ctx, token)
	if err != nil {
		slog.Warn("qq: get gateway failed", "err", err)
		return err
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	slog.Info("qq: gateway connected")

	return q.gatewayLoop(ctx, conn, token)
}

func (q *QQChannel) getAccessToken(ctx context.Context) (string, error) {
	q.tokenMu.Lock()
	defer q.tokenMu.Unlock()
	if q.token != "" && time.Now().Before(q.tokenExp) {
		return q.token, nil
	}
	body := map[string]string{
		"appId":        q.cfg.AppID,
		"clientSecret": q.cfg.Secret,
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://bots.qq.com/app/getAppAccessToken", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}
	_ = json.Unmarshal(b, &result)
	if result.AccessToken == "" {
		return "", fmt.Errorf("qq: get token failed: %s", string(b))
	}
	q.token = result.AccessToken
	q.tokenExp = time.Now().Add(7100 * time.Second) // tokens last ~7200s
	return q.token, nil
}

func (q *QQChannel) getGatewayURL(ctx context.Context, token string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.sgroup.qq.com/gateway", nil)
	req.Header.Set("Authorization", "QQBot "+token)
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		URL string `json:"url"`
	}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &result)
	if result.URL == "" {
		return "", fmt.Errorf("qq: no gateway url: %s", string(b))
	}
	return result.URL, nil
}

func (q *QQChannel) gatewayLoop(ctx context.Context, conn *websocket.Conn, token string) error {
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
			T  string          `json:"t"`
			S  int             `json:"s"`
			D  json.RawMessage `json:"d"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}

		switch payload.Op {
		case 10: // HELLO
			var hello struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			_ = json.Unmarshal(payload.D, &hello)
			go q.heartbeatLoop(ctx, conn, time.Duration(hello.HeartbeatInterval)*time.Millisecond, heartbeatStop)
			if err := q.identify(conn, token); err != nil {
				return err
			}
		case 0:
			if payload.T == "C2C_MESSAGE_CREATE" {
				var msg map[string]any
				_ = json.Unmarshal(payload.D, &msg)
				go q.handleC2CMessage(msg)
			}
		}
	}
}

func (q *QQChannel) heartbeatLoop(ctx context.Context, conn *websocket.Conn, interval time.Duration, stop <-chan struct{}) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			data, _ := json.Marshal(map[string]any{"op": 1, "d": nil})
			_ = conn.WriteMessage(websocket.TextMessage, data)
		case <-stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (q *QQChannel) identify(conn *websocket.Conn, token string) error {
	payload := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   "QQBot " + token,
			"intents": 1 << 25, // C2C_MESSAGE_CREATE
			"shard":   []int{0, 1},
		},
	}
	data, _ := json.Marshal(payload)
	return conn.WriteMessage(websocket.TextMessage, data)
}

func (q *QQChannel) handleC2CMessage(payload map[string]any) {
	msgID, _ := payload["id"].(string)

	q.seenMu.Lock()
	if q.seen[msgID] {
		q.seenMu.Unlock()
		return
	}
	q.seen[msgID] = true
	q.seenQueue = append(q.seenQueue, msgID)
	if len(q.seenQueue) > 1000 {
		del := q.seenQueue[0]
		q.seenQueue = q.seenQueue[1:]
		delete(q.seen, del)
	}
	q.seenMu.Unlock()

	author, _ := payload["author"].(map[string]any)
	senderID, _ := author["user_openid"].(string)
	if senderID == "" {
		senderID, _ = author["id"].(string)
	}
	content, _ := payload["content"].(string)
	if content == "" || senderID == "" {
		return
	}

	q.HandleMessage(senderID, senderID, content, nil, map[string]any{
		"message_id": msgID,
	})
}

func (q *QQChannel) Send(ctx context.Context, msg bus.ChannelMessage) error {
	token, err := q.getAccessToken(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{
		"content":  msg.Content(),
		"msg_type": 0,
	}
	if mid, ok := msg.Metadata()["message_id"].(string); ok {
		body["msg_id"] = mid
	}
	data, _ := json.Marshal(body)
	url := fmt.Sprintf("https://api.sgroup.qq.com/v2/users/%s/messages", msg.ChatId())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "QQBot "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
