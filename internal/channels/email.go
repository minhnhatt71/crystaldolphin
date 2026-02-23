package channels

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"time"

	"github.com/crystaldolphin/crystaldolphin/internal/bus"
	"github.com/crystaldolphin/crystaldolphin/internal/config"
)

// EmailChannel polls IMAP for new messages and sends via SMTP.
// Uses stdlib net/smtp for sending; polls IMAP via raw IMAP4 commands
// to avoid bringing in a heavy dependency.
type EmailChannel struct {
	Base
	cfg     *config.EmailConfig
	seenUID map[uint32]bool
}

func NewEmailChannel(cfg *config.EmailConfig, b *bus.MessageBus) *EmailChannel {
	return &EmailChannel{
		Base:    NewBase("email", b, cfg.AllowFrom),
		cfg:     cfg,
		seenUID: make(map[uint32]bool),
	}
}

func (e *EmailChannel) Name() string { return "email" }

func (e *EmailChannel) Start(ctx context.Context) error {
	if !e.cfg.ConsentGranted {
		slog.Warn("email: consent_granted is false — channel disabled until consent is set")
		<-ctx.Done()
		return ctx.Err()
	}
	if e.cfg.IMAPHost == "" {
		slog.Warn("email: imapHost not configured")
		<-ctx.Done()
		return ctx.Err()
	}

	interval := time.Duration(e.cfg.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	slog.Info("email: polling started", "host", e.cfg.IMAPHost, "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := e.poll(ctx); err != nil {
				slog.Warn("email: poll error", "err", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// poll connects to IMAP, fetches unseen messages, dispatches them, marks seen.
func (e *EmailChannel) poll(ctx context.Context) error {
	addr := net.JoinHostPort(e.cfg.IMAPHost, fmt.Sprintf("%d", e.cfg.IMAPPort))

	var conn net.Conn
	var err error
	if e.cfg.IMAPUseSSL {
		tlsCfg := &tls.Config{ServerName: e.cfg.IMAPHost}
		conn, err = tls.Dial("tcp", addr, tlsCfg)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 15*time.Second)
	}
	if err != nil {
		return fmt.Errorf("imap connect: %w", err)
	}
	defer conn.Close()

	imap := newIMAPConn(conn)

	// Read server greeting.
	if _, err := imap.readline(); err != nil {
		return err
	}

	// LOGIN
	if err := imap.cmd("A1", fmt.Sprintf("LOGIN %q %q", e.cfg.IMAPUsername, e.cfg.IMAPPassword)); err != nil {
		return fmt.Errorf("imap login: %w", err)
	}

	// SELECT mailbox
	mailbox := e.cfg.IMAPMailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if err := imap.cmd("A2", fmt.Sprintf("SELECT %q", mailbox)); err != nil {
		return fmt.Errorf("imap select: %w", err)
	}

	// SEARCH UNSEEN
	lines, err := imap.search("A3", "SEARCH UNSEEN")
	if err != nil {
		return err
	}

	var seqNums []string
	for _, line := range lines {
		if strings.HasPrefix(line, "* SEARCH") {
			parts := strings.Fields(line)
			for _, p := range parts[2:] {
				seqNums = append(seqNums, p)
			}
		}
	}

	for _, seq := range seqNums {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rawMsg, err := imap.fetch("A4"+seq, seq, "(RFC822)")
		if err != nil {
			slog.Warn("email: fetch error", "seq", seq, "err", err)
			continue
		}
		from, subject, body := parseEmail(rawMsg)
		if from == "" {
			continue
		}

		senderID := extractEmail(from)
		if !e.IsAllowed(senderID) {
			continue
		}

		maxChars := e.cfg.MaxBodyChars
		if maxChars <= 0 {
			maxChars = 12000
		}
		if len(body) > maxChars {
			body = body[:maxChars]
		}

		content := fmt.Sprintf("Subject: %s\nFrom: %s\n\n%s", subject, from, body)

		e.HandleMessage(senderID, senderID, content, nil, map[string]any{
			"from":    from,
			"subject": subject,
			"seq":     seq,
		})

		if e.cfg.MarkSeen {
			_ = imap.cmd("A5"+seq, fmt.Sprintf("STORE %s +FLAGS (\\Seen)", seq))
		}
	}

	_ = imap.cmd("A99", "LOGOUT")
	return nil
}

func (e *EmailChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	to := msg.ChatID
	subject := e.cfg.SubjectPrefix + "Message"
	if s, ok := msg.Metadata["subject"].(string); ok && s != "" {
		subject = e.cfg.SubjectPrefix + s
	}

	body := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		to, e.cfg.FromAddress, subject, msg.Content)

	addr := net.JoinHostPort(e.cfg.SMTPHost, fmt.Sprintf("%d", e.cfg.SMTPPort))
	auth := smtp.PlainAuth("", e.cfg.SMTPUsername, e.cfg.SMTPPassword, e.cfg.SMTPHost)

	var err error
	if e.cfg.SMTPUseSSL {
		tlsCfg := &tls.Config{ServerName: e.cfg.SMTPHost}
		conn, dialErr := tls.Dial("tcp", addr, tlsCfg)
		if dialErr != nil {
			return dialErr
		}
		client, _ := smtp.NewClient(conn, e.cfg.SMTPHost)
		if err = client.Auth(auth); err != nil {
			return err
		}
		if err = client.Mail(e.cfg.FromAddress); err != nil {
			return err
		}
		if err = client.Rcpt(to); err != nil {
			return err
		}
		w, _ := client.Data()
		_, err = w.Write([]byte(body))
		_ = w.Close()
		client.Quit()
	} else {
		err = smtp.SendMail(addr, auth, e.cfg.FromAddress, []string{to}, []byte(body))
	}
	return err
}

// ---------------------------------------------------------------------------
// Minimal IMAP client (avoids importing emersion/go-imap just for polling)
// ---------------------------------------------------------------------------

type imapConn struct {
	conn net.Conn
	buf  strings.Builder
}

func newIMAPConn(conn net.Conn) *imapConn { return &imapConn{conn: conn} }

func (c *imapConn) readline() (string, error) {
	var b [1]byte
	for {
		_, err := c.conn.Read(b[:])
		if err != nil {
			return c.buf.String(), err
		}
		if b[0] == '\n' {
			line := c.buf.String()
			c.buf.Reset()
			return strings.TrimRight(line, "\r"), nil
		}
		c.buf.WriteByte(b[0])
	}
}

func (c *imapConn) cmd(tag, command string) error {
	_, err := fmt.Fprintf(c.conn, "%s %s\r\n", tag, command)
	if err != nil {
		return err
	}
	for {
		line, err := c.readline()
		if err != nil {
			return err
		}
		if strings.HasPrefix(line, tag+" OK") {
			return nil
		}
		if strings.HasPrefix(line, tag+" NO") || strings.HasPrefix(line, tag+" BAD") {
			return fmt.Errorf("imap: %s", line)
		}
	}
}

func (c *imapConn) search(tag, command string) ([]string, error) {
	_, err := fmt.Fprintf(c.conn, "%s %s\r\n", tag, command)
	if err != nil {
		return nil, err
	}
	var lines []string
	for {
		line, err := c.readline()
		if err != nil {
			return lines, err
		}
		lines = append(lines, line)
		if strings.HasPrefix(line, tag+" OK") || strings.HasPrefix(line, tag+" NO") || strings.HasPrefix(line, tag+" BAD") {
			return lines, nil
		}
	}
}

func (c *imapConn) fetch(tag, seq, items string) (string, error) {
	_, err := fmt.Fprintf(c.conn, "%s FETCH %s %s\r\n", tag, seq, items)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	inBody := false
	for {
		line, err := c.readline()
		if err != nil {
			return sb.String(), err
		}
		if strings.HasPrefix(line, "* "+seq+" FETCH") {
			inBody = true
			continue
		}
		if inBody {
			if strings.HasPrefix(line, tag+" OK") {
				break
			}
			if line == ")" {
				break
			}
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String(), nil
}

// ---------------------------------------------------------------------------
// Email parsing helpers
// ---------------------------------------------------------------------------

var reFrom = regexp.MustCompile(`(?i)^From:\s*(.+)$`)
var reSubj = regexp.MustCompile(`(?i)^Subject:\s*(.+)$`)
var reTags = regexp.MustCompile(`<[^>]+>`)
var reMultiNL = regexp.MustCompile(`\n{3,}`)

func parseEmail(raw string) (from, subject, body string) {
	lines := strings.Split(raw, "\n")
	var bodyLines []string
	inBody := false
	for _, line := range lines {
		if inBody {
			bodyLines = append(bodyLines, line)
			continue
		}
		if line == "" || line == "\r" {
			inBody = true
			continue
		}
		if m := reFrom.FindStringSubmatch(line); m != nil {
			from = strings.TrimSpace(m[1])
		}
		if m := reSubj.FindStringSubmatch(line); m != nil {
			subject = strings.TrimSpace(m[1])
		}
	}
	rawBody := strings.Join(bodyLines, "\n")
	// Strip HTML tags.
	rawBody = reTags.ReplaceAllString(rawBody, "")
	rawBody = reMultiNL.ReplaceAllString(rawBody, "\n\n")
	body = strings.TrimSpace(rawBody)
	return
}

func extractEmail(from string) string {
	// "Name <email@host>" → "email@host"
	start := strings.LastIndex(from, "<")
	end := strings.LastIndex(from, ">")
	if start >= 0 && end > start {
		return from[start+1 : end]
	}
	return strings.TrimSpace(from)
}
