package channels

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
	channelmodel "github.com/crystaldolphin/crystaldolphin/internal/modeling/channel"
)

// Compile-time assertion: Manager satisfies the domain interface.
var _ channelmodel.Manager = (*Manager)(nil)

// Manager owns all enabled channels and routes outbound messages.
type Manager struct {
	channels map[channelmodel.ChannelName]channelmodel.Channel
	msgBus   *bus.MessageBus
}

// NewManager creates a Manager and initialises all enabled channels.
func NewManager(cfg *config.Config, msgBus *bus.MessageBus) *Manager {
	m := &Manager{
		channels: make(map[channelmodel.ChannelName]channelmodel.Channel),
		msgBus:   msgBus,
	}

	cli := NewCLIChannel(msgBus.Inbound(), msgBus.Outbound())
	m.Register(cli)
	slog.Info("channel enabled", "name", cli.Name())

	if cfg.Channels.Telegram.Enabled {
		ch := NewTelegramChannel(&cfg.Channels.Telegram, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}
	if cfg.Channels.WhatsApp.Enabled {
		ch := NewWhatsAppChannel(&cfg.Channels.WhatsApp, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}
	if cfg.Channels.Discord.Enabled {
		ch := NewDiscordChannel(&cfg.Channels.Discord, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}
	if cfg.Channels.Slack.Enabled {
		ch := NewSlackChannel(&cfg.Channels.Slack, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}
	if cfg.Channels.Feishu.Enabled {
		ch := NewFeishuChannel(&cfg.Channels.Feishu, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}
	if cfg.Channels.DingTalk.Enabled {
		ch := NewDingTalkChannel(&cfg.Channels.DingTalk, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}
	if cfg.Channels.Email.Enabled {
		ch := NewEmailChannel(&cfg.Channels.Email, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}
	if cfg.Channels.Mochat.Enabled {
		ch := NewMochatChannel(&cfg.Channels.Mochat, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}
	if cfg.Channels.QQ.Enabled {
		ch := NewQQChannel(&cfg.Channels.QQ, msgBus.Inbound())
		m.Register(ch)
		slog.Info("channel enabled", "name", ch.Name())
	}

	return m
}

// Register adds a Channel to the manager's registry.
func (m *Manager) Register(ch channelmodel.Channel) {
	m.channels[ch.Name()] = ch
}

// EnabledChannels returns the names of all enabled channels.
func (m *Manager) EnabledChannels() []channelmodel.ChannelName {
	names := make([]channelmodel.ChannelName, 0, len(m.channels))
	for n := range m.channels {
		names = append(names, n)
	}
	return names
}

// Start starts all channels concurrently and dispatches outbound messages.
// Blocks until ctx is cancelled.
func (m *Manager) Start(ctx context.Context) error {
	go m.dispatchOutbound(ctx)

	for name, ch := range m.channels {
		go func(n channelmodel.ChannelName, c channelmodel.Channel) {
			slog.Info("starting channel", "name", n)
			if err := c.Start(ctx); err != nil && ctx.Err() == nil {
				slog.Error("channel exited with error", "name", n, "err", err)
			}
		}(name, ch)
	}

	<-ctx.Done()
	return ctx.Err()
}

// Send routes msg to the channel identified by channelName.
func (m *Manager) Send(ctx context.Context, channelName string, msg channelmodel.Message) error {
	ch, ok := m.channels[channelmodel.ChannelName(channelName)]
	if !ok {
		return fmt.Errorf("channels: unknown channel %q", channelName)
	}
	return ch.Send(ctx, msg)
}

// dispatchOutbound reads OutboundMessages from the bus and routes each to the
// appropriate channel's Send method.
func (m *Manager) dispatchOutbound(ctx context.Context) {
	for {
		select {
		case busMsg := <-m.msgBus.Outbound().Subscribe():
			ch, ok := m.channels[busMsg.ChannelName()]
			if !ok {
				slog.Debug("unknown channel for outbound message", "channel", busMsg.ChannelName())
				continue
			}
			if err := ch.Send(ctx, busMsg.ToChannelMessage()); err != nil {
				slog.Error("send error", "channel", busMsg.ChannelName(), "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
