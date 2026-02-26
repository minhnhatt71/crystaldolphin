package channels

import (
	"context"
	"log/slog"
	"regexp"
	"strings"

	slackgo "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config/channel"
)

// SlackChannel implements Slack via Socket Mode.
type SlackChannel struct {
	Base
	cfg       *channel.SlackConfig
	webClient *slackgo.Client
	smClient  *socketmode.Client
	botUserID string
}

func NewSlackChannel(cfg *channel.SlackConfig, b bus.Bus) *SlackChannel {
	return &SlackChannel{
		Base: NewBase("slack", b, nil), // Slack uses its own allow logic
		cfg:  cfg,
	}
}

func (s *SlackChannel) Name() string { return "slack" }

func (s *SlackChannel) Start(ctx context.Context) error {
	if s.cfg.BotToken == "" || s.cfg.AppToken == "" {
		slog.Warn("slack: bot/app token not configured")
		<-ctx.Done()
		return ctx.Err()
	}

	s.webClient = slackgo.New(s.cfg.BotToken,
		slackgo.OptionAppLevelToken(s.cfg.AppToken))

	// Resolve bot user ID.
	if resp, err := s.webClient.AuthTestContext(ctx); err == nil {
		s.botUserID = resp.UserID
		slog.Info("slack: connected", "bot_user_id", s.botUserID)
	}

	s.smClient = socketmode.New(s.webClient)

	go s.smClient.RunContext(ctx) //nolint:errcheck

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-s.smClient.Events:
			if !ok {
				return nil
			}
			s.handleEvent(ctx, evt)
		}
	}
}

func (s *SlackChannel) handleEvent(_ context.Context, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		s.smClient.Ack(*evt.Request)
		cb, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		if cb.InnerEvent.Type != "message" && cb.InnerEvent.Type != "app_mention" {
			return
		}
		// Inner event data is map[string]interface{} — parse manually.
		s.handleInnerEvent(cb.InnerEvent)
	}
}

func (s *SlackChannel) handleInnerEvent(ev slackevents.EventsAPIInnerEvent) {
	data, ok := ev.Data.(map[string]interface{})
	if !ok {
		return
	}
	userID, _ := data["user"].(string)
	channel, _ := data["channel"].(string)
	text, _ := data["text"].(string)
	subtype, _ := data["subtype"].(string)
	channelType, _ := data["channel_type"].(string)
	ts, _ := data["ts"].(string)
	threadTS, _ := data["thread_ts"].(string)

	if subtype != "" || userID == "" || channel == "" {
		return
	}
	if userID == s.botUserID {
		return
	}
	// Avoid double-processing mention + message events.
	if ev.Type == "message" && s.botUserID != "" && strings.Contains(text, "<@"+s.botUserID+">") {
		return
	}

	if !s.isAllowedSlack(userID, channel, channelType) {
		return
	}
	if channelType != "im" && !s.shouldRespond(ev.Type, text, channel) {
		return
	}

	text = s.stripMention(text)

	if s.cfg.ReplyInThread && threadTS == "" {
		threadTS = ts
	}

	// Best-effort reaction.
	if s.webClient != nil && ts != "" {
		_ = s.webClient.AddReaction(s.cfg.ReactEmoji, slackgo.ItemRef{
			Channel:   channel,
			Timestamp: ts,
		})
	}

	s.HandleMessage(userID, channel, text, nil, map[string]any{
		"slack": map[string]any{
			"thread_ts":    threadTS,
			"channel_type": channelType,
		},
	})
}

func (s *SlackChannel) isAllowedSlack(user, channel, channelType string) bool {
	if channelType == "im" {
		if !s.cfg.DM.Enabled {
			return false
		}
		if s.cfg.DM.Policy == "allowlist" {
			for _, a := range s.cfg.DM.AllowFrom {
				if a == user {
					return true
				}
			}
			return false
		}
		return true
	}
	if s.cfg.GroupPolicy == "allowlist" {
		for _, a := range s.cfg.GroupAllowFrom {
			if a == channel {
				return true
			}
		}
		return false
	}
	return true
}

func (s *SlackChannel) shouldRespond(evType, text, channel string) bool {
	switch s.cfg.GroupPolicy {
	case "open":
		return true
	case "mention":
		if evType == "app_mention" {
			return true
		}
		return s.botUserID != "" && strings.Contains(text, "<@"+s.botUserID+">")
	case "allowlist":
		for _, a := range s.cfg.GroupAllowFrom {
			if a == channel {
				return true
			}
		}
		return false
	}
	return false
}

func (s *SlackChannel) stripMention(text string) string {
	if s.botUserID == "" {
		return text
	}
	re := regexp.MustCompile(`<@` + regexp.QuoteMeta(s.botUserID) + `>\s*`)
	return strings.TrimSpace(re.ReplaceAllString(text, ""))
}

func (s *SlackChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if s.webClient == nil {
		return nil
	}
	slack := map[string]any{}
	if m, ok := msg.Metadata()["slack"].(map[string]any); ok {
		slack = m
	}
	threadTS, _ := slack["thread_ts"].(string)
	channelType, _ := slack["channel_type"].(string)

	var options []slackgo.MsgOption
	options = append(options, slackgo.MsgOptionText(msg.Content(), false))
	if threadTS != "" && channelType != "im" {
		options = append(options, slackgo.MsgOptionTS(threadTS))
	}

	_, _, err := s.webClient.PostMessageContext(ctx, msg.ChatID(), options...)
	return err
}
