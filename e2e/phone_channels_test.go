//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests talk to the real BulkGate and Meta Graph APIs over the
// internet; there is no emulator for either.
//
// The "rejects" tests need no account: they send with invalid credentials and
// check the gateway understands the providers' real error responses and does
// not retry them. Set E2E_OFFLINE=1 to skip them without internet.
//
// The "live" tests deliver a real message and only run with credentials:
//
//	E2E_BULKGATE_APPLICATION_ID, E2E_BULKGATE_APPLICATION_TOKEN, E2E_SMS_TO
//	  (BulkGate's 14-day trial is free; E2E_SMS_TO is an E.164 number)
//	E2E_WHATSAPP_PHONE_NUMBER_ID, E2E_WHATSAPP_ACCESS_TOKEN, E2E_WHATSAPP_TO
//	  (a Meta developer app's free test number can send to up to five
//	  allowlisted recipients; by default the test sends the pre-approved
//	  hello_world template. E2E_WHATSAPP_TEMPLATE, E2E_WHATSAPP_LANGUAGE and
//	  E2E_WHATSAPP_BODY_VARIABLES select your own template instead.)

func requireInternet(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E_OFFLINE") != "" {
		t.Skip("E2E_OFFLINE is set")
	}
}

// phoneSetup starts a gateway with the given notification YAML, creates a
// contact with phone, and returns the admin client and the contact's ID.
func phoneSetup(t *testing.T, notificationYAML, phone string) (*client, string) {
	t.Helper()
	gw := startGateway(t, baseConfig("notification:\n"+notificationYAML))
	admin := newClient(t, gw.URL)
	admin.login(adminUsername, adminPassword)
	email := uniqueEmail("phone")
	user := admin.sendJSON(http.MethodPost, "/_/api/users", map[string]any{"username": email, "email": email, "phone": phone}, http.StatusCreated)
	return admin, user["id"].(string)
}

// sendAndWait creates a notification on channel and waits for its first
// delivery attempt to finish.
func sendAndWait(t *testing.T, admin *client, userID, channel string) map[string]any {
	t.Helper()
	created := admin.sendJSON(http.MethodPost, "/_/api/notifications", map[string]any{
		"userIds": []string{userID}, "type": "e2e", "title": "Taronja e2e",
		"body":     "Test message sent at " + time.Now().Format(time.RFC3339),
		"actions":  []map[string]any{{"id": "ok", "label": "OK"}},
		"channels": []string{channel},
	}, http.StatusCreated)
	id := created["notifications"].([]any)[0].(map[string]any)["id"].(string)

	var delivery map[string]any
	eventually(t, 30*time.Second, "the "+channel+" delivery to finish", func() bool {
		deliveries := admin.getJSON("/_/api/notifications/"+id+"/deliveries", http.StatusOK)["deliveries"].([]any)
		if len(deliveries) == 0 {
			return false
		}
		delivery = deliveries[0].(map[string]any)
		return delivery["status"] != "pending"
	})
	return delivery
}

func TestBulkGate_RealAPIRejectsBadCredentials(t *testing.T) {
	requireInternet(t)
	admin, userID := phoneSetup(t, `  sms:
    enabled: true
    bulkgate:
      applicationId: "0"
      applicationToken: invalid-token
`, "+34600000000")

	delivery := sendAndWait(t, admin, userID, "sms")
	assert.Equal(t, "failed", delivery["status"])
	assert.Contains(t, delivery["error"], "unknown_identity", "BulkGate's real error is parsed")
	assert.Contains(t, delivery["error"], "not retried")
	assert.Nil(t, delivery["nextRetryAt"], "bad credentials are not retried")
}

func TestMeta_RealAPIRejectsBadToken(t *testing.T) {
	requireInternet(t)
	admin, userID := phoneSetup(t, `  whatsapp:
    enabled: true
    phoneNumberId: "000000000000000"
    accessToken: invalid-token
    template:
      name: hello_world
      language: en_US
      bodyVariables: 0
`, "+34600000000")

	delivery := sendAndWait(t, admin, userID, "whatsapp")
	assert.Equal(t, "failed", delivery["status"])
	assert.Contains(t, delivery["error"], "code 190", "Meta's real OAuthException is parsed")
	assert.Nil(t, delivery["nextRetryAt"], "an invalid token is not retried")
}

func requireEnv(t *testing.T, names ...string) map[string]string {
	t.Helper()
	values := map[string]string{}
	var missing []string
	for _, name := range names {
		values[name] = os.Getenv(name)
		if values[name] == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("live test: set %s to run it", strings.Join(missing, ", "))
	}
	return values
}

func TestBulkGate_LiveDelivery(t *testing.T) {
	env := requireEnv(t, "E2E_BULKGATE_APPLICATION_ID", "E2E_BULKGATE_APPLICATION_TOKEN", "E2E_SMS_TO")
	admin, userID := phoneSetup(t, `  sms:
    enabled: true
    bulkgate:
      applicationId: "`+env["E2E_BULKGATE_APPLICATION_ID"]+`"
      applicationToken: "`+env["E2E_BULKGATE_APPLICATION_TOKEN"]+`"
    maxSegments: 1
`, env["E2E_SMS_TO"])

	delivery := sendAndWait(t, admin, userID, "sms")
	require.Equal(t, "sent", delivery["status"], "error: %v", delivery["error"])
	t.Logf("BulkGate accepted the SMS to %s; check the phone", env["E2E_SMS_TO"])
}

func TestWhatsApp_LiveDelivery(t *testing.T) {
	env := requireEnv(t, "E2E_WHATSAPP_PHONE_NUMBER_ID", "E2E_WHATSAPP_ACCESS_TOKEN", "E2E_WHATSAPP_TO")
	template := envOr("E2E_WHATSAPP_TEMPLATE", "hello_world")
	language := envOr("E2E_WHATSAPP_LANGUAGE", "en_US")
	variables := envOr("E2E_WHATSAPP_BODY_VARIABLES", "0")
	admin, userID := phoneSetup(t, `  whatsapp:
    enabled: true
    phoneNumberId: "`+env["E2E_WHATSAPP_PHONE_NUMBER_ID"]+`"
    accessToken: "`+env["E2E_WHATSAPP_ACCESS_TOKEN"]+`"
    template:
      name: `+template+`
      language: `+language+`
      bodyVariables: `+variables+`
`, env["E2E_WHATSAPP_TO"])

	delivery := sendAndWait(t, admin, userID, "whatsapp")
	require.Equal(t, "sent", delivery["status"], "error: %v", delivery["error"])
	t.Logf("Meta accepted the %s template to %s; check WhatsApp", template, env["E2E_WHATSAPP_TO"])
}
