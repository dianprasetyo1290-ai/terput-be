package otp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SMTPProvider emails the code. Email only: a phone destination is refused,
// since this fork delivers OTP over email and never WhatsApp/SMS.
type SMTPProvider struct {
	Host, Port         string
	User, Password     string
	FromName, FromAddr string
	SchoolName         string
}

// Send delivers the code over SMTP with STARTTLS (port 587 style).
func (p SMTPProvider) Send(ctx context.Context, destination, code string) error {
	school := p.SchoolName
	if school == "" {
		school = "PPDB"
	}
	return p.SendMail(ctx, destination, fmt.Sprintf("Kode verifikasi PPDB %s: %s", school, code), otpBody(school, code))
}

// SendMail delivers one plain-text email over SMTP with STARTTLS. It is shared
// by the OTP sender and the guardian notification outbox.
func (p SMTPProvider) SendMail(ctx context.Context, destination, subject, body string) error {
	to, err := mail.ParseAddress(destination)
	if err != nil || !strings.Contains(destination, "@") {
		return errors.New("smtp provider only delivers to email addresses")
	}
	if p.Host == "" || p.FromAddr == "" {
		return errors.New("smtp provider not configured")
	}

	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(p.Host, p.Port))
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, p.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.StartTLS(&tls.Config{ServerName: p.Host}); err != nil {
		return err
	}
	if p.User != "" {
		if err := c.Auth(smtp.PlainAuth("", p.User, p.Password, p.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(p.FromAddr); err != nil {
		return err
	}
	if err := c.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(p.message(to.Address, subject, body)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func otpBody(school, code string) string {
	return fmt.Sprintf("Kode verifikasi PPDB %s Anda:\r\n\r\n    %s\r\n\r\n"+
		"Kode berlaku 5 menit dan hanya bisa dipakai sekali.\r\n"+
		"Jangan berikan kode ini kepada siapa pun, termasuk pihak sekolah.\r\n"+
		"Abaikan email ini jika Anda tidak sedang mendaftar.\r\n", school, code)
}

func (p SMTPProvider) message(to, subject, body string) []byte {
	from := (&mail.Address{Name: p.FromName, Address: p.FromAddr}).String()
	return []byte("From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" + body)
}
