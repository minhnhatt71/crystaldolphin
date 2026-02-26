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

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config/channel"
)

// MochatChannel polls the Mochat HTTP API for new messages.
// The full Python implementation uses Socket.IO; here we use HTTP polling
// as a simpler, dependency-free approach that preserves all behaviour.
type MochatChannel struct {
	Base
	cfg        *channel.MochatConfig
	httpClient *http.Client
	mu         sync.Mutex
	cursors    map[string]string // sessionID/panelID → cursor
	seen       map[string]bool   // dedup message IDs (bounded to 1000)
	seenQueue  []string
}

func NewMochatChannel(cfg *channel.MochatConfig, b bus.Bus) *MochatChannel {
	return &MochatChannel{
		Base:       NewBase("mochat", b, cfg.AllowFrom),
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cursors:    make(map[string]string),
		seen:       make(map[string]bool),
	}
}

func (m *MochatChannel) Name() string { return "mochat" }

func (m *MochatChannel) Start(ctx context.Context) error {
	if m.cfg.ClawToken == "" || m.cfg.BaseURL == "" {
		slog.Warn("mochat: clawToken or baseUrl not configured")
		<-ctx.Done()
		return ctx.Err()
	}

	interval := time.Duration(m.cfg.RefreshIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	slog.Info("mochat: polling started", "interval", interval)

	for {
		select {
		case <-ticker.C:
			if err := m.poll(ctx); err != nil {
				slog.Warn("mochat: poll error", "err", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (m *MochatChannel) poll(ctx context.Context) error {
	// Poll sessions.
	for _, sessID := range m.cfg.Sessions {
		msgs, cursor, err := m.fetchMessages(ctx, "session", sessID)
		if err != nil {
			slog.Warn("mochat: fetch session error", "id", sessID, "err", err)
			continue
		}
		m.mu.Lock()
		m.cursors["session:"+sessID] = cursor
		m.mu.Unlock()
		for _, msg := range msgs {
			m.dispatch(sessID, msg)
		}
	}
	// Poll panels.
	for _, panelID := range m.cfg.Panels {
		msgs, cursor, err := m.fetchMessages(ctx, "panel", panelID)
		if err != nil {
			slog.Warn("mochat: fetch panel error", "id", panelID, "err", err)
			continue
		}
		m.mu.Lock()
		m.cursors["panel:"+panelID] = cursor
		m.mu.Unlock()
		for _, msg := range msgs {
			m.dispatch(panelID, msg)
		}
	}
	return nil
}

type mochatMsg struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	SenderID  string `json:"sender_id"`
	CreatedAt string `json:"created_at"`
}

func (m *MochatChannel) fetchMessages(ctx context.Context, kind, id string) ([]mochatMsg, string, error) {
	m.mu.Lock()
	cursor := m.cursors[kind+":"+id]
	m.mu.Unlock()

	url := fmt.Sprintf("%s/api/messages?type=%s&id=%s&limit=%d", m.cfg.BaseURL, kind, id, m.cfg.WatchLimit)
	if cursor != "" {
		url += "&cursor=" + cursor
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.ClawToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var result struct {
		Messages []mochatMsg `json:"messages"`
		Cursor   string      `json:"cursor"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, cursor, err
	}
	return result.Messages, result.Cursor, nil
}

func (m *MochatChannel) dispatch(chatID string, msg mochatMsg) {
	m.mu.Lock()
	if m.seen[msg.ID] {
		m.mu.Unlock()
		return
	}
	m.seen[msg.ID] = true
	m.seenQueue = append(m.seenQueue, msg.ID)
	if len(m.seenQueue) > 1000 {
		del := m.seenQueue[0]
		m.seenQueue = m.seenQueue[1:]
		delete(m.seen, del)
	}
	m.mu.Unlock()

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return
	}
	m.HandleMessage(msg.SenderID, chatID, content, nil, map[string]any{
		"message_id": msg.ID,
		"created_at": msg.CreatedAt,
	})
}

func (m *MochatChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	url := m.cfg.BaseURL + "/api/messages/send"
	body := map[string]any{
		"session_id": msg.ChatId(),
		"content":    msg.Content(),
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.ClawToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
