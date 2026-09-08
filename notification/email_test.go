package notification

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSMTPServer is a minimal real SMTP server — no auth, no STARTTLS
// advertised — good enough to exercise net/smtp.SendMail's actual wire
// protocol (EHLO/MAIL FROM/RCPT TO/DATA) end to end, the same "fake but
// real protocol" testing style this codebase uses elsewhere (e.g. the fake
// OTLP collector in gateway/tracing_test.go, the fake JWKS server in
// providers/apple_test.go) rather than mocking net/smtp's internals.
type fakeSMTPServer struct {
	listener net.Listener
	mailFrom string
	rcptTo   string
	data     string
}

func newFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &fakeSMTPServer{listener: ln}
	go s.serveOne(t)
	return s
}

func (s *fakeSMTPServer) addr() (host string, port int) {
	tcpAddr := s.listener.Addr().(*net.TCPAddr)
	return tcpAddr.IP.String(), tcpAddr.Port
}

func (s *fakeSMTPServer) serveOne(t *testing.T) {
	conn, err := s.listener.Accept()
	if err != nil {
		return // listener closed
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := conn

	write := func(line string) {
		_, _ = w.Write([]byte(line + "\r\n"))
	}
	write("220 fake.smtp ESMTP")

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
			write("250 fake.smtp greets you")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.mailFrom = line[len("MAIL FROM:"):]
			write("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.rcptTo = line[len("RCPT TO:"):]
			write("250 OK")
		case upper == "DATA":
			write("354 Start mail input; end with <CRLF>.<CRLF>")
			var sb strings.Builder
			for {
				dataLine, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dataLine, "\r\n") == "." {
					break
				}
				sb.WriteString(dataLine)
			}
			s.data = sb.String()
			write("250 OK: queued")
		case upper == "QUIT":
			write("221 Bye")
			return
		default:
			write("250 OK")
		}
	}
}

func (s *fakeSMTPServer) close() { s.listener.Close() }

func TestEmailProvider(t *testing.T) {
	t.Run("NewEmailProvider returns nil when not configured", func(t *testing.T) {
		assert.Nil(t, NewEmailProvider(config.EmailNotificationConfig{}))
		assert.Nil(t, NewEmailProvider(config.EmailNotificationConfig{Enabled: false, Host: "x", From: "a@b.com"}))
		assert.Nil(t, NewEmailProvider(config.EmailNotificationConfig{Enabled: true, From: "a@b.com"})) // no host
	})

	t.Run("Send delivers a real SMTP message with the notification content", func(t *testing.T) {
		server := newFakeSMTPServer(t)
		defer server.close()
		host, port := server.addr()

		provider := NewEmailProvider(config.EmailNotificationConfig{
			Enabled: true, Host: host, Port: port, From: "gateway@example.com", FromName: "Taronja Gateway",
		})
		require.NotNil(t, provider)
		assert.Equal(t, "email", provider.Channel())

		notif := &db.Notification{Title: "Track added", Body: "A new track was added to your folder."}
		externalRef, err := provider.Send(context.Background(), SendRequest{
			Notification: notif,
			Recipient:    "parent@example.com",
		})
		require.NoError(t, err)
		assert.Empty(t, externalRef, "email has nothing analogous to an editable sent message")

		// Give the fake server's goroutine a moment to finish recording
		// after SendMail returns (it returns once QUIT's response is
		// read, so this is really just yielding the scheduler).
		time.Sleep(20 * time.Millisecond)

		assert.Contains(t, server.mailFrom, "gateway@example.com")
		assert.Contains(t, server.rcptTo, "parent@example.com")
		assert.Contains(t, server.data, "Subject: Track added")
		assert.Contains(t, server.data, "A new track was added to your folder.")
		assert.Contains(t, server.data, "From: Taronja Gateway <gateway@example.com>")
	})

	t.Run("Send includes action links when the notification has actions and a respond token", func(t *testing.T) {
		server := newFakeSMTPServer(t)
		defer server.close()
		host, port := server.addr()

		provider := NewEmailProvider(config.EmailNotificationConfig{
			Enabled: true, Host: host, Port: port, From: "gateway@example.com",
		})
		require.NotNil(t, provider)

		notif := &db.Notification{Title: "Approve request", Body: "Please respond."}
		_, err := provider.Send(context.Background(), SendRequest{
			Notification:   notif,
			Recipient:      "parent@example.com",
			Actions:        []Action{{ID: "approve", Label: "Approve"}, {ID: "deny", Label: "Deny"}},
			RespondBaseURL: "https://gw.example.com/_/notifications/respond",
			RespondToken:   "opaque-token-123",
		})
		require.NoError(t, err)
		time.Sleep(20 * time.Millisecond)

		assert.Contains(t, server.data, "Approve: https://gw.example.com/_/notifications/respond?token=opaque-token-123&action=approve")
		assert.Contains(t, server.data, "Deny: https://gw.example.com/_/notifications/respond?token=opaque-token-123&action=deny")
	})

	t.Run("Send fails when nothing is listening", func(t *testing.T) {
		// Port 1 is reserved/unassigned, near-guaranteed to have nothing
		// listening in any test environment, and fails fast (connection
		// refused) rather than needing a timeout.
		provider := NewEmailProvider(config.EmailNotificationConfig{
			Enabled: true, Host: "127.0.0.1", Port: 1, From: "gateway@example.com",
		})
		require.NotNil(t, provider)
		_, err := provider.Send(context.Background(), SendRequest{
			Notification: &db.Notification{Title: "x", Body: "y"},
			Recipient:    "parent@example.com",
		})
		assert.Error(t, err)
	})
}
