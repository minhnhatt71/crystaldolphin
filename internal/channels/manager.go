package channels

import (
	"context"
	"log/slog"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	"github.com/crystaldolphin/crystaldolphin/internal/schema"
)

// Manager owns all enabled channels and routes outbound messages.
type Manager struct {
	channels   map[string]schema.Channel
	channelBus *bus.ChannelBus
}

// NewManager creates a Manager and initialises all enabled channels.
// The CLIChannel is always registered; it uses consoleBus to deliver replies
// back to the terminal when the gateway is running interactively.
func NewManager(cfg *config.Config, inbound *bus.AgentBus, outbound *bus.ChannelBus, console *bus.ConsoleBus) *Manager {
	m := &Manager{
		channels:   make(map[string]schema.Channel),
		channelBus: outbound,
	}

	cli := NewCLIChannel(inbound, console)
	m.channels[cli.Name()] = cli
	slog.Info("channel enabled", "name", cli.Name())

	if cfg.Channels.Telegram.Enabled {
		ch := NewTelegramChannel(&cfg.Channels.Telegram, inbound)
		m.channels["telegram"] = ch
		slog.Info("channel enabled", "name", "telegram")
	}
	if cfg.Channels.WhatsApp.Enabled {
		ch := NewWhatsAppChannel(&cfg.Channels.WhatsApp, inbound)
		m.channels["whatsapp"] = ch
		slog.Info("channel enabled", "name", "whatsapp")
	}
	if cfg.Channels.Discord.Enabled {
		ch := NewDiscordChannel(&cfg.Channels.Discord, inbound)
		m.channels["discord"] = ch
		slog.Info("channel enabled", "name", "discord")
	}
	if cfg.Channels.Slack.Enabled {
		ch := NewSlackChannel(&cfg.Channels.Slack, inbound)
		m.channels["slack"] = ch
		slog.Info("channel enabled", "name", "slack")
	}
	if cfg.Channels.Feishu.Enabled {
		ch := NewFeishuChannel(&cfg.Channels.Feishu, inbound)
		m.channels["feishu"] = ch
		slog.Info("channel enabled", "name", "feishu")
	}
	if cfg.Channels.DingTalk.Enabled {
		ch := NewDingTalkChannel(&cfg.Channels.DingTalk, inbound)
		m.channels["dingtalk"] = ch
		slog.Info("channel enabled", "name", "dingtalk")
	}
	if cfg.Channels.Email.Enabled {
		ch := NewEmailChannel(&cfg.Channels.Email, inbound)
		m.channels["email"] = ch
		slog.Info("channel enabled", "name", "email")
	}
	if cfg.Channels.Mochat.Enabled {
		ch := NewMochatChannel(&cfg.Channels.Mochat, inbound)
		m.channels["mochat"] = ch
		slog.Info("channel enabled", "name", "mochat")
	}
	if cfg.Channels.QQ.Enabled {
		ch := NewQQChannel(&cfg.Channels.QQ, inbound)
		m.channels["qq"] = ch
		slog.Info("channel enabled", "name", "qq")
	}

	return m
}

// EnabledChannels returns the names of all enabled channels.
func (m *Manager) EnabledChannels() []string {
	names := make([]string, 0, len(m.channels))
	for n := range m.channels {
		names = append(names, n)
	}
	return names
}

// StartAll starts all channels concurrently and dispatches outbound messages.
// Blocks until ctx is cancelled.
func (m *Manager) StartAll(ctx context.Context) error {
	// Start outbound dispatcher.
	go m.dispatchOutbound(ctx)

	// Start each channel in its own goroutine.
	for name, ch := range m.channels {
		go func(n string, c schema.Channel) {
			slog.Info("starting channel", "name", n)
			if err := c.Start(ctx); err != nil && ctx.Err() == nil {
				slog.Error("channel exited with error", "name", n, "err", err)
			}
		}(name, ch)
	}

	<-ctx.Done()
	return ctx.Err()
}

// dispatchOutbound reads from bus.Outbound and routes each message to the
// appropriate channel's Send method.
func (m *Manager) dispatchOutbound(ctx context.Context) {
	for {
		select {
		case msg := <-m.channelBus.Subscribe():
			ch, ok := m.channels[string(msg.Channel())]
			if !ok {
				slog.Debug("unknown channel for outbound message", "channel", msg.Channel())
				continue
			}
			if err := ch.Send(ctx, msg); err != nil {
				slog.Error("send error", "channel", msg.Channel(), "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
