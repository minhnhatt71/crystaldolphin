package channels

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config/channel"
)

// TelegramChannel implements the Telegram bot via long polling.
type TelegramChannel struct {
	Base
	cfg *channel.TelegramConfig
	bot *tgbotapi.BotAPI
}

// NewTelegramChannel creates a TelegramChannel.
func NewTelegramChannel(cfg *channel.TelegramConfig, b bus.Bus) *TelegramChannel {
	return &TelegramChannel{
		Base: NewBase("telegram", b, cfg.AllowFrom),
		cfg:  cfg,
	}
}

func (t *TelegramChannel) Name() string { return "telegram" }

func (t *TelegramChannel) Start(ctx context.Context) error {
	if t.cfg.Token == "" {
		return fmt.Errorf("telegram: bot token not configured")
	}
	bot, err := tgbotapi.NewBotAPI(t.cfg.Token)
	if err != nil {
		return fmt.Errorf("telegram: create bot: %w", err)
	}
	t.bot = bot
	slog.Info("telegram: connected", "username", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := bot.GetUpdatesChan(u)

	for {
		select {
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			go t.handleUpdate(ctx, update)
		case <-ctx.Done():
			bot.StopReceivingUpdates()
			return ctx.Err()
		}
	}
}

func (t *TelegramChannel) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}

	senderID := fmt.Sprintf("%d", msg.From.ID)
	if msg.From.UserName != "" {
		senderID = senderID + "|" + msg.From.UserName
	}
	chatID := fmt.Sprintf("%d", msg.Chat.ID)

	content := msg.Text
	if msg.Caption != "" {
		content = msg.Caption
	}

	var mediaPaths []string
	if msg.Photo != nil {
		photo := msg.Photo[len(msg.Photo)-1]
		if path, err := t.downloadFile(photo.FileID, ".jpg"); err == nil {
			mediaPaths = append(mediaPaths, path)
			content = strings.TrimSpace(content + "\n[image: " + path + "]")
		}
	}
	if msg.Document != nil {
		if path, err := t.downloadFile(msg.Document.FileID, ""); err == nil {
			mediaPaths = append(mediaPaths, path)
			content = strings.TrimSpace(content + "\n[file: " + path + "]")
		}
	}

	if content == "" {
		content = "[empty message]"
	}

	// Start typing indicator.
	typingCtx, cancelTyping := context.WithCancel(ctx)
	defer cancelTyping()
	go t.sendTypingLoop(typingCtx, msg.Chat.ID)

	metadata := map[string]any{
		"message_id": msg.MessageID,
		"user_id":    msg.From.ID,
		"username":   msg.From.UserName,
		"first_name": msg.From.FirstName,
		"is_group":   msg.Chat.Type != "private",
	}

	t.HandleMessage(senderID, chatID, content, mediaPaths, metadata)
}

func (t *TelegramChannel) downloadFile(fileID, ext string) (string, error) {
	if t.bot == nil {
		return "", fmt.Errorf("bot not running")
	}
	file, err := t.bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	mediaDir := filepath.Join(home, ".nanobot", "media")
	_ = os.MkdirAll(mediaDir, 0o755)
	if ext == "" {
		ext = filepath.Ext(file.FilePath)
	}
	dest := filepath.Join(mediaDir, fileID[:min(16, len(fileID))]+ext)
	url := file.Link(t.cfg.Token)
	if err := downloadToFileTG(url, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func downloadToFileTG(url, dest string) error {
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

func (t *TelegramChannel) sendTypingLoop(ctx context.Context, chatID int64) {
	for {
		if t.bot != nil {
			action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
			_, _ = t.bot.Send(action)
		}
		select {
		case <-time.After(4 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

func (t *TelegramChannel) Send(_ context.Context, msg bus.OutboundMessage) error {
	if t.bot == nil {
		return fmt.Errorf("telegram: bot not running")
	}
	chatID, err := parseChatID(msg.ChatID())
	if err != nil {
		return err
	}

	// Send media files first.
	for _, mediaPath := range msg.Media() {
		f, err := os.Open(mediaPath)
		if err != nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(mediaPath))
		var sendCfg tgbotapi.Chattable
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
			p := tgbotapi.NewPhoto(chatID, tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(mediaPath)).Media)
			sendCfg = p
		default:
			sendCfg = tgbotapi.NewDocument(chatID, tgbotapi.FileReader{Name: filepath.Base(mediaPath), Reader: f})
		}
		_ = f.Close()
		_, _ = t.bot.Send(sendCfg)
	}

	if msg.Content() == "" || msg.Content() == "[empty message]" {
		return nil
	}

	// Get optional reply-to message ID.
	var replyMsgID int
	if t.cfg.ReplyToMessage {
		if mid, ok := msg.Metadata()["message_id"]; ok {
			switch v := mid.(type) {
			case int:
				replyMsgID = v
			case float64:
				replyMsgID = int(v)
			}
		}
	}

	for _, chunk := range splitMessage(msg.Content(), 4000) {
		html := markdownToTelegramHTML(chunk)
		m := tgbotapi.NewMessage(chatID, html)
		m.ParseMode = "HTML"
		if replyMsgID != 0 {
			m.ReplyToMessageID = replyMsgID
		}
		if _, err := t.bot.Send(m); err != nil {
			// Fallback to plain text.
			m2 := tgbotapi.NewMessage(chatID, chunk)
			if replyMsgID != 0 {
				m2.ReplyToMessageID = replyMsgID
			}
			_, _ = t.bot.Send(m2)
		}
	}
	return nil
}

func parseChatID(s string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid chat_id: %s", s)
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// Markdown → Telegram HTML converter (mirrors Python _markdown_to_telegram_html)
// ---------------------------------------------------------------------------

var (
	reTGCodeBlock  = regexp.MustCompile("(?s)```[\\w]*\\n?([\\s\\S]*?)```")
	reTGInlineCode = regexp.MustCompile("`([^`]+)`")
	reTGHeader     = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reTGBlockquote = regexp.MustCompile(`(?m)^>\s*(.*)$`)
	reTGLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reTGBold1      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reTGBold2      = regexp.MustCompile(`__(.+?)__`)
	reTGItalic     = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])_([^_]+)_(?:[^a-zA-Z0-9]|$)`)
	reTGStrike     = regexp.MustCompile(`~~(.+?)~~`)
	reTGBullet     = regexp.MustCompile(`(?m)^[-*]\s+`)
)

func markdownToTelegramHTML(text string) string {
	if text == "" {
		return ""
	}

	// 1. Extract code blocks.
	var codeBlocks []string
	text = reTGCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		groups := reTGCodeBlock.FindStringSubmatch(m)
		codeBlocks = append(codeBlocks, groups[1])
		return fmt.Sprintf("\x00CB%d\x00", len(codeBlocks)-1)
	})

	// 2. Extract inline code.
	var inlineCodes []string
	text = reTGInlineCode.ReplaceAllStringFunc(text, func(m string) string {
		groups := reTGInlineCode.FindStringSubmatch(m)
		inlineCodes = append(inlineCodes, groups[1])
		return fmt.Sprintf("\x00IC%d\x00", len(inlineCodes)-1)
	})

	// 3. Strip headers.
	text = reTGHeader.ReplaceAllString(text, "$1")
	// 4. Strip blockquotes.
	text = reTGBlockquote.ReplaceAllString(text, "$1")

	// 5. HTML escape.
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")

	// 6. Links.
	text = reTGLink.ReplaceAllString(text, `<a href="$2">$1</a>`)
	// 7. Bold.
	text = reTGBold1.ReplaceAllString(text, "<b>$1</b>")
	text = reTGBold2.ReplaceAllString(text, "<b>$1</b>")
	// 8. Italic.
	text = reTGItalic.ReplaceAllString(text, "<i>$1</i>")
	// 9. Strikethrough.
	text = reTGStrike.ReplaceAllString(text, "<s>$1</s>")
	// 10. Bullet lists.
	text = reTGBullet.ReplaceAllString(text, "• ")

	// 11. Restore inline code.
	for i, code := range inlineCodes {
		escaped := htmlEscape(code)
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00IC%d\x00", i),
			"<code>"+escaped+"</code>")
	}
	// 12. Restore code blocks.
	for i, code := range codeBlocks {
		escaped := htmlEscape(code)
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00CB%d\x00", i),
			"<pre><code>"+escaped+"</code></pre>")
	}
	return text
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
