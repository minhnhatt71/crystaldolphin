package channel

// EmailConfig configures the email channel (IMAP inbound + SMTP outbound).
type EmailConfig struct {
	Enabled        bool `json:"enabled"`
	ConsentGranted bool `json:"consentGranted"`

	// IMAP (receive)
	IMAPHost     string `json:"imapHost"`
	IMAPPort     int    `json:"imapPort"`
	IMAPUsername string `json:"imapUsername"`
	IMAPPassword string `json:"imapPassword"`
	IMAPMailbox  string `json:"imapMailbox"`
	IMAPUseSSL   bool   `json:"imapUseSsl"`

	// SMTP (send)
	SMTPHost     string `json:"smtpHost"`
	SMTPPort     int    `json:"smtpPort"`
	SMTPUsername string `json:"smtpUsername"`
	SMTPPassword string `json:"smtpPassword"`
	SMTPUseTLS   bool   `json:"smtpUseTls"`
	SMTPUseSSL   bool   `json:"smtpUseSsl"`
	FromAddress  string `json:"fromAddress"`

	// Behaviour
	AutoReplyEnabled    bool     `json:"autoReplyEnabled"`
	PollIntervalSeconds int      `json:"pollIntervalSeconds"`
	MarkSeen            bool     `json:"markSeen"`
	MaxBodyChars        int      `json:"maxBodyChars"`
	SubjectPrefix       string   `json:"subjectPrefix"`
	AllowFrom           []string `json:"allowFrom"`
}

func DefaultEmailConfig() EmailConfig {
	return EmailConfig{
		IMAPPort:            993,
		IMAPMailbox:         "INBOX",
		IMAPUseSSL:          true,
		SMTPPort:            587,
		SMTPUseTLS:          true,
		AutoReplyEnabled:    true,
		PollIntervalSeconds: 30,
		MarkSeen:            true,
		MaxBodyChars:        12000,
		SubjectPrefix:       "Re: ",
		AllowFrom:           []string{},
	}
}
