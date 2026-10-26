//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	mailpitURL      = envOr("E2E_MAILPIT_URL", "http://localhost:18025")
	mailpitSMTPPort = envOr("E2E_MAILPIT_SMTP_PORT", "11025")
)

type mailpitAddress struct {
	Name    string
	Address string
}

type mailpitMessage struct {
	ID      string
	Subject string
	From    mailpitAddress
	To      []mailpitAddress
	Text    string
}

// mailpitMessagesTo returns the messages Mailpit holds for one recipient, in
// summary form (no body).
func mailpitMessagesTo(t *testing.T, recipient string) []mailpitMessage {
	t.Helper()
	resp, err := http.Get(mailpitURL + "/api/v1/search?query=" + url.QueryEscape(fmt.Sprintf("to:%q", recipient)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var result struct{ Messages []mailpitMessage }
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result.Messages
}

// mailpitMessage returns one message with its body.
func mailpitGetMessage(t *testing.T, id string) mailpitMessage {
	t.Helper()
	resp, err := http.Get(mailpitURL + "/api/v1/message/" + id)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var msg mailpitMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&msg))
	return msg
}

func requireMailpit(t *testing.T) {
	t.Helper()
	resp, err := http.Get(mailpitURL + "/readyz")
	if err != nil {
		t.Fatalf("Mailpit is not reachable at %s (start it with `make e2e-up`): %v", mailpitURL, err)
	}
	resp.Body.Close()
}

func emailConfig() string {
	return baseConfig(`authenticationProviders:
  basic:
    enabled: true
notification:
  email:
    enabled: true
    host: localhost
    port: ` + mailpitSMTPPort + `
    username: e2e
    password: e2e-smtp-password
    from: noreply@taronja.test
    fromName: Taronja E2E
`)
}

// A notification created through the API reaches the user's inbox over SMTP,
// and clicking an action link in that email records the answer.
func TestEmailNotification_DeliveredAndAnsweredFromEmail(t *testing.T) {
	requireMailpit(t)
	gw := startGateway(t, emailConfig())
	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)

	suffix := fmt.Sprint(time.Now().UnixNano())
	username := "bob-" + suffix
	email := username + "@example.com"
	user := admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{
		"username": username, "email": email, "password": "bob-password-123",
	}, http.StatusCreated)
	userID := user["id"].(string)

	const title = "Deploy approval needed ✅ café"
	created := admin.sendJSON(http.MethodPost, "/_/api/notifications", map[string]any{
		"userIds":  []string{userID},
		"type":     "deploy_approval",
		"title":    title,
		"body":     "Release v1.2.3 is ready to ship.",
		"url":      "https://example.com/releases/v1.2.3",
		"channels": []string{"email"},
		"actions": []map[string]any{
			{"id": "approve", "label": "Approve"},
			{"id": "reject", "label": "Reject"},
		},
	}, http.StatusCreated)
	notificationID := created["notifications"].([]any)[0].(map[string]any)["id"].(string)

	var summary mailpitMessage
	eventually(t, 15*time.Second, "the email to arrive in Mailpit", func() bool {
		messages := mailpitMessagesTo(t, email)
		if len(messages) == 0 {
			return false
		}
		summary = messages[0]
		return true
	})
	require.Len(t, mailpitMessagesTo(t, email), 1, "exactly one email per notification")

	msg := mailpitGetMessage(t, summary.ID)
	assert.Equal(t, title, msg.Subject, "non-ASCII subjects must survive the trip")
	assert.Equal(t, mailpitAddress{Name: "Taronja E2E", Address: "noreply@taronja.test"}, msg.From)
	require.Len(t, msg.To, 1)
	assert.Equal(t, email, msg.To[0].Address)
	assert.Contains(t, msg.Text, "Release v1.2.3 is ready to ship.")
	assert.Contains(t, msg.Text, "https://example.com/releases/v1.2.3")

	status := admin.getJSON("/_/api/notifications/"+notificationID+"/status", http.StatusOK)
	assert.Equal(t, "sent", status["overall"])
	assert.Equal(t, map[string]any{"email": "sent"}, status["channels"])

	approveLink := regexp.MustCompile(`Approve: (\S+)`).FindStringSubmatch(msg.Text)
	require.NotNil(t, approveLink, "email should carry an Approve link:\n%s", msg.Text)
	require.Contains(t, approveLink[1], gw.URL+"/_/api/notifications/respond?")

	// The link is opened from a mail client: no session cookie.
	resp, err := http.Get(approveLink[1])
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	bob := newClient(t, gw.URL)
	bob.login(username, "bob-password-123")
	list := bob.getJSON("/_/api/notifications", http.StatusOK)
	notifications := list["notifications"].([]any)
	require.Len(t, notifications, 1)
	answered := notifications[0].(map[string]any)
	assert.Equal(t, "approve", answered["respondedActionId"])
	assert.Equal(t, "email", answered["respondedVia"])

	// A second click must not change the recorded answer.
	rejectLink := regexp.MustCompile(`Reject: (\S+)`).FindStringSubmatch(msg.Text)
	require.NotNil(t, rejectLink)
	resp, err = http.Get(rejectLink[1])
	require.NoError(t, err)
	resp.Body.Close()
	list = bob.getJSON("/_/api/notifications", http.StatusOK)
	assert.Equal(t, "approve", list["notifications"].([]any)[0].(map[string]any)["respondedActionId"])
}

// A notification for several users sends one separate email to each.
func TestEmailNotification_OneEmailPerRecipient(t *testing.T) {
	requireMailpit(t)
	gw := startGateway(t, emailConfig())
	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)

	suffix := fmt.Sprint(time.Now().UnixNano())
	var userIDs, emails []string
	for _, name := range []string{"carol", "dave", "erin"} {
		username := name + "-" + suffix
		email := username + "@example.com"
		user := admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{
			"username": username, "email": email, "password": "password-123456",
		}, http.StatusCreated)
		userIDs = append(userIDs, user["id"].(string))
		emails = append(emails, email)
	}

	created := admin.sendJSON(http.MethodPost, "/_/api/notifications", map[string]any{
		"userIds":  userIDs,
		"type":     "announcement",
		"title":    "Maintenance tonight",
		"body":     "The service will be down for 5 minutes at 23:00.",
		"channels": []string{"email"},
	}, http.StatusCreated)

	for _, email := range emails {
		eventually(t, 15*time.Second, "an email for "+email, func() bool {
			return len(mailpitMessagesTo(t, email)) == 1
		})
		msg := mailpitMessagesTo(t, email)[0]
		require.Len(t, msg.To, 1, "recipients must not see each other")
		assert.Equal(t, email, msg.To[0].Address)
		assert.Equal(t, "Maintenance tonight", msg.Subject)
	}

	batch := admin.getJSON("/_/api/notifications/batches/"+created["batchId"].(string)+"/status", http.StatusOK)
	assert.EqualValues(t, 3, batch["total"])
	assert.EqualValues(t, 3, batch["sent"])
}
