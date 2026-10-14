package provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// ChannelRouter sends each channel through its own adapter, falling back to
// Default (the NOTIFICATION_PROVIDER adapter) for channels without one.
type ChannelRouter struct {
	Default  Provider
	Email    Provider
	WhatsApp Provider
}

// Name implements Provider.
func (r ChannelRouter) Name() string {
	name := func(p Provider) string {
		if p == nil {
			return r.Default.Name()
		}
		return p.Name()
	}
	return fmt.Sprintf("email=%s,whatsapp=%s", name(r.Email), name(r.WhatsApp))
}

// Send implements Provider.
func (r ChannelRouter) Send(ctx context.Context, msg Message) (string, error) {
	switch {
	case msg.Channel == "email" && r.Email != nil:
		return r.Email.Send(ctx, msg)
	case msg.Channel == "whatsapp" && r.WhatsApp != nil:
		return r.WhatsApp.Send(ctx, msg)
	}
	return r.Default.Send(ctx, msg)
}

// SMTPConfig configures the email adapter (env SMTP_*).
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// ImplicitTLS dials TLS directly (port 465); otherwise STARTTLS is used
	// when the server offers it, and required when credentials are set.
	ImplicitTLS bool
	Timeout     time.Duration
}

// SMTPProvider delivers email over SMTP (BUG-029).
type SMTPProvider struct{ Cfg SMTPConfig }

// NewSMTP validates the configuration.
func NewSMTP(cfg SMTPConfig) (*SMTPProvider, error) {
	if cfg.Host == "" || cfg.Port <= 0 || cfg.From == "" {
		return nil, fmt.Errorf("SMTP_HOST, SMTP_PORT dan SMTP_FROM wajib diisi untuk NOTIFICATION_EMAIL_PROVIDER=smtp")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &SMTPProvider{Cfg: cfg}, nil
}

// Name implements Provider.
func (*SMTPProvider) Name() string { return "smtp" }

// Send implements Provider. The receipt is the generated Message-ID.
func (p *SMTPProvider) Send(ctx context.Context, msg Message) (string, error) {
	if strings.ContainsAny(msg.Recipient, "\r\n") || !strings.Contains(msg.Recipient, "@") {
		return "", fmt.Errorf("alamat email tidak valid")
	}
	addr := net.JoinHostPort(p.Cfg.Host, fmt.Sprint(p.Cfg.Port))
	dialer := &net.Dialer{Timeout: p.Cfg.Timeout}
	var conn net.Conn
	var err error
	if p.Cfg.ImplicitTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: p.Cfg.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return "", fmt.Errorf("smtp connect: %w", err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(p.Cfg.Timeout))
	}
	c, err := smtp.NewClient(conn, p.Cfg.Host)
	if err != nil {
		conn.Close()
		return "", fmt.Errorf("smtp handshake: %w", err)
	}
	defer c.Close()
	if !p.Cfg.ImplicitTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: p.Cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return "", fmt.Errorf("smtp starttls: %w", err)
			}
		} else if p.Cfg.Username != "" {
			return "", fmt.Errorf("smtp server tidak mendukung STARTTLS; kredensial tidak dikirim tanpa enkripsi")
		}
	}
	if p.Cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", p.Cfg.Username, p.Cfg.Password, p.Cfg.Host)); err != nil {
			return "", fmt.Errorf("smtp auth: %w", err)
		}
	}
	msgID := fmt.Sprintf("<%s@%s>", randomID()+randomID(), p.Cfg.Host)
	if err := c.Mail(p.Cfg.From); err != nil {
		return "", fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(msg.Recipient); err != nil {
		return "", fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return "", fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(buildEmail(p.Cfg.From, msg, msgID)); err != nil {
		return "", fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("smtp send: %w", err)
	}
	_ = c.Quit()
	return msgID, nil
}

func buildEmail(from string, msg Message, msgID string) []byte {
	var b bytes.Buffer
	subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(msg.Subject)
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: %s\r\nDate: %s\r\n",
		from, msg.Recipient, mime.QEncoding.Encode("utf-8", subject), msgID, time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	body := strings.ReplaceAll(strings.ReplaceAll(msg.Body, "\r\n", "\n"), "\n", "\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return b.Bytes()
}

// HTTPWhatsAppConfig configures a generic HTTP WhatsApp gateway (env WA_*).
type HTTPWhatsAppConfig struct {
	URL     string // POST endpoint of the gateway
	Token   string // sent as "Authorization: Bearer <token>"
	Timeout time.Duration
}

// HTTPWhatsApp sends free-text WhatsApp notifications through a gateway that
// accepts POST {"to":"<phone>","message":"<text>"} and answers 2xx with
// {"id":"..."} or {"message_id":"..."} (BUG-029).
type HTTPWhatsApp struct {
	Cfg  HTTPWhatsAppConfig
	HTTP *http.Client
}

// NewHTTPWhatsApp validates the configuration.
func NewHTTPWhatsApp(cfg HTTPWhatsAppConfig) (*HTTPWhatsApp, error) {
	if !strings.HasPrefix(cfg.URL, "https://") && !strings.HasPrefix(cfg.URL, "http://") {
		return nil, fmt.Errorf("WA_API_URL wajib berupa URL http(s) untuk NOTIFICATION_WHATSAPP_PROVIDER=http")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("WA_API_TOKEN wajib diisi untuk NOTIFICATION_WHATSAPP_PROVIDER=http")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &HTTPWhatsApp{Cfg: cfg, HTTP: &http.Client{Timeout: cfg.Timeout}}, nil
}

// Name implements Provider.
func (*HTTPWhatsApp) Name() string { return "http-whatsapp" }

// NormalizePhone turns a local Indonesian number into international digits
// (08xx -> 628xx, +62 -> 62). Returns "" when it is not a plausible number.
func NormalizePhone(v string) string {
	var d strings.Builder
	for _, r := range v {
		if r >= '0' && r <= '9' {
			d.WriteRune(r)
		}
	}
	s := d.String()
	if strings.HasPrefix(s, "0") {
		s = "62" + s[1:]
	}
	if len(s) < 9 || len(s) > 15 {
		return ""
	}
	return s
}

// Send implements Provider.
func (p *HTTPWhatsApp) Send(ctx context.Context, msg Message) (string, error) {
	to := NormalizePhone(msg.Recipient)
	if to == "" {
		return "", fmt.Errorf("nomor WhatsApp tidak valid")
	}
	text := msg.Body
	if msg.Subject != "" {
		text = "*" + msg.Subject + "*\n" + msg.Body
	}
	raw, _ := json.Marshal(map[string]string{"to": to, "message": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Cfg.URL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.Cfg.Token)
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("whatsapp gateway: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("whatsapp gateway menjawab %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		ID        string `json:"id"`
		MessageID string `json:"message_id"`
	}
	_ = json.Unmarshal(body, &out)
	if out.MessageID != "" {
		return out.MessageID, nil
	}
	if out.ID != "" {
		return out.ID, nil
	}
	return "", fmt.Errorf("whatsapp gateway tidak mengembalikan id pesan")
}
