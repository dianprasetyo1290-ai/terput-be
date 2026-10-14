package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizePhone(t *testing.T) {
	for in, want := range map[string]string{
		"0812-3456-7890": "6281234567890", "+62 812 3456 7890": "6281234567890",
		"6281234567890": "6281234567890", "123": "", "": "",
	} {
		if got := NormalizePhone(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestHTTPWhatsApp(t *testing.T) {
	var got map[string]string
	var auth string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"message_id":"wa-123"}`))
	}))
	defer srv.Close()
	wa, err := NewHTTPWhatsApp(HTTPWhatsAppConfig{URL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := wa.Send(context.Background(), Message{Channel: "whatsapp", Recipient: "0812 3456 7890", Subject: "Tagihan", Body: "Rp100.000"})
	if err != nil || id != "wa-123" {
		t.Fatalf("send: id=%q err=%v", id, err)
	}
	if auth != "Bearer tok" || got["to"] != "6281234567890" || !strings.Contains(got["message"], "Rp100.000") {
		t.Fatalf("request: auth=%q body=%v", auth, got)
	}
	status = http.StatusUnauthorized
	if _, err := wa.Send(context.Background(), Message{Channel: "whatsapp", Recipient: "081234567890", Body: "x"}); err == nil {
		t.Fatal("gateway 401 must be an error, never a fake success")
	}
	if _, err := wa.Send(context.Background(), Message{Channel: "whatsapp", Recipient: "12", Body: "x"}); err == nil {
		t.Fatal("invalid phone must fail")
	}
}

func TestNewProviderConfigErrors(t *testing.T) {
	if _, err := NewHTTPWhatsApp(HTTPWhatsAppConfig{URL: "ftp://x", Token: "t"}); err == nil {
		t.Error("bad url accepted")
	}
	if _, err := NewHTTPWhatsApp(HTTPWhatsAppConfig{URL: "https://x"}); err == nil {
		t.Error("missing token accepted")
	}
	if _, err := NewSMTP(SMTPConfig{Host: "smtp.x", Port: 587}); err == nil {
		t.Error("missing from accepted")
	}
}

func TestChannelRouter_UnconfiguredRefuses(t *testing.T) {
	r := ChannelRouter{Default: UnconfiguredProvider{}}
	if _, err := r.Send(context.Background(), Message{Channel: "whatsapp", Recipient: "0812"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

// fakeSMTP is a minimal plaintext SMTP server on a random local port.
func fakeSMTP(t *testing.T) (host string, port int, received chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	received = make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		rd := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 fake ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					received <- data.String()
					write("250 OK queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				write("250 fake")
			case strings.HasPrefix(cmd, "DATA"):
				inData = true
				write("354 go ahead")
			case strings.HasPrefix(cmd, "QUIT"):
				write("221 bye")
				return
			default:
				write("250 OK")
			}
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, received
}

func TestSMTPProvider(t *testing.T) {
	host, port, received := fakeSMTP(t)
	p, err := NewSMTP(SMTPConfig{Host: host, Port: port, From: "sekolah@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := p.Send(context.Background(), Message{Channel: "email", Recipient: "wali@example.com", Subject: "Tagihan baru terbit", Body: "Tagihan Oktober"})
	if err != nil || id == "" {
		t.Fatalf("send: id=%q err=%v", id, err)
	}
	msg := <-received
	for _, want := range []string{"To: wali@example.com", "Subject: Tagihan baru terbit", "Tagihan Oktober", "Message-ID: " + id} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if _, err := p.Send(context.Background(), Message{Channel: "email", Recipient: "bukan-email"}); err == nil {
		t.Fatal("invalid address accepted")
	}
}

// TestSMTPProvider_RefusesPlaintextAuth: credentials never travel without TLS.
func TestSMTPProvider_RefusesPlaintextAuth(t *testing.T) {
	host, port, _ := fakeSMTP(t)
	p, _ := NewSMTP(SMTPConfig{Host: host, Port: port, From: "a@b.c", Username: "u", Password: "p"})
	if _, err := p.Send(context.Background(), Message{Channel: "email", Recipient: "x@y.z", Body: "b"}); err == nil {
		t.Fatal("auth without STARTTLS must fail")
	}
}

func TestDescribeChannel(t *testing.T) {
	tests := []struct {
		name, provider, wantState, wantProvider string
		wantConfigured                          bool
	}{
		{"empty", "", ChannelStateUnconfigured, "unconfigured", false},
		{"unconfigured", "unconfigured", ChannelStateUnconfigured, "unconfigured", false},
		{"disabled", "disabled", ChannelStateUnconfigured, "unconfigured", false},
		{"dev noop", "dev-noop", ChannelStateSimulated, "dev-noop", false},
		{"noop", "noop", ChannelStateSimulated, "noop", false},
		{"smtp", "smtp", ChannelStateActive, "smtp", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DescribeChannel("email", tt.provider)
			if got.State != tt.wantState || got.Provider != tt.wantProvider || got.Configured != tt.wantConfigured || got.Channel != "email" {
				t.Errorf("DescribeChannel(%q) = %+v", tt.provider, got)
			}
		})
	}
}

func TestChannelRouter_For(t *testing.T) {
	smtp := NoopProvider{}
	r := ChannelRouter{Default: UnconfiguredProvider{}, Email: smtp}
	if r.For("email") != Provider(smtp) || r.For("whatsapp") != Provider(UnconfiguredProvider{}) {
		t.Fatal("For must pick the channel provider, else the default")
	}
}
