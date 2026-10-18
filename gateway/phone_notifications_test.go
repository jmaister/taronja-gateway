package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingAPI stands in for BulkGate or Meta: it keeps each JSON body and
// answers with a fixed success response.
type recordingAPI struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
}

func newRecordingAPI(t *testing.T, response string) *recordingAPI {
	api := &recordingAPI{}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		api.mu.Lock()
		api.bodies = append(api.bodies, body)
		api.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	t.Cleanup(api.Close)
	return api
}

func (a *recordingAPI) waitForOne(t *testing.T) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		n := len(a.bodies)
		a.mu.Unlock()
		if n > 0 {
			a.mu.Lock()
			defer a.mu.Unlock()
			return a.bodies[n-1]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the API was never called")
	return nil
}

type phoneGateway struct {
	t      *testing.T
	server *httptest.Server
	admin  *http.Cookie
}

func newPhoneGateway(t *testing.T, notificationCfg config.NotificationConfig) *phoneGateway {
	t.Helper()
	d := deps.NewTestWithName(t.Name())
	server := httptest.NewUnstartedServer(nil)
	cfg := &config.GatewayConfig{
		Server:       config.ServerConfig{Host: "localhost", URL: "http://" + server.Listener.Addr().String()},
		Management:   config.ManagementConfig{Session: config.SessionConfig{DurationSeconds: 3600}},
		Notification: notificationCfg,
	}
	shutdown, err := InitNotifications(context.Background(), cfg.Notification, cfg.Server, d)
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	gw, err := NewGatewayWithDependencies(cfg, nil, d)
	require.NoError(t, err)
	server.Config.Handler = gw.Server.Handler
	server.Start()
	t.Cleanup(server.Close)

	admin := &db.User{Username: "school-admin", Email: "admin@school.example", Provider: db.AdminProvider, EmailConfirmed: true}
	require.NoError(t, d.UserRepo.CreateUser(admin))
	sess, err := d.SessionStore.NewSession(nil, admin, "basic", time.Hour)
	require.NoError(t, err)
	return &phoneGateway{t: t, server: server, admin: &http.Cookie{Name: session.SessionCookieName, Value: sess.Token}}
}

func (g *phoneGateway) call(method, path string, body any, wantStatus int) map[string]any {
	g.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(g.t, err)
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, g.server.URL+path, reader)
	require.NoError(g.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(g.admin)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(g.t, err)
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	require.Equal(g.t, wantStatus, resp.StatusCode, "%s %s: %s", method, path, data)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func TestPhoneNotifications_SMSThroughTheAPI(t *testing.T) {
	bulkgate := newRecordingAPI(t, `{"data":{"status":"accepted","sms_id":"sms-1","number":"34600123456"}}`)
	gw := newPhoneGateway(t, config.NotificationConfig{SMS: config.SMSNotificationConfig{
		Enabled: true, BulkGate: config.BulkGateConfig{ApplicationID: "app", ApplicationToken: "tok", APIURL: bulkgate.URL},
	}})

	// A parent who never logs in: no password, a phone number.
	parent := gw.call(http.MethodPost, "/_/api/users", map[string]any{"username": "parent-1", "email": "parent1@example.com", "phone": "+34 600 123 456"}, http.StatusCreated)
	parentID := parent["id"].(string)
	assert.Equal(t, "+34600123456", parent["phone"], "normalized to E.164")
	assert.Equal(t, false, parent["phoneVerified"])

	gw.call(http.MethodPatch, "/_/api/users/"+parentID, map[string]any{"phone": "600123456"}, http.StatusBadRequest)
	gw.call(http.MethodPut, "/_/api/users/"+parentID+"/notifications/preferences", map[string]any{"preferredChannel": "pigeon"}, http.StatusBadRequest)
	pref := gw.call(http.MethodPut, "/_/api/users/"+parentID+"/notifications/preferences", map[string]any{"preferredChannel": "sms"}, http.StatusOK)
	assert.Equal(t, "sms", pref["preferredChannel"])

	statusReq, err := http.NewRequest(http.MethodGet, gw.server.URL+"/_/api/users/"+parentID+"/notifications/channels/status", nil)
	require.NoError(t, err)
	statusReq.AddCookie(gw.admin)
	statusResp, err := http.DefaultClient.Do(statusReq)
	require.NoError(t, err)
	var statuses []map[string]any
	require.NoError(t, json.NewDecoder(statusResp.Body).Decode(&statuses))
	statusResp.Body.Close()
	require.Len(t, statuses, 1)
	assert.Equal(t, "sms", statuses[0]["channel"])
	assert.Equal(t, "connected", statuses[0]["status"])

	// The app sends as for every channel: no channel named, the preference picks SMS.
	created := gw.call(http.MethodPost, "/_/api/notifications", map[string]any{
		"userIds": []string{parentID}, "type": "trip", "title": "Excursion", "body": "Tomorrow at 9.",
		"actions": []map[string]any{{"id": "yes", "label": "Will attend"}, {"id": "no", "label": "Will not attend"}},
	}, http.StatusCreated)
	notificationID := created["notifications"].([]any)[0].(map[string]any)["id"].(string)

	sms := bulkgate.waitForOne(t)
	assert.Equal(t, "34600123456", sms["number"])
	link := regexp.MustCompile(`http://\S+/_/n/\S+`).FindString(sms["text"].(string))
	require.NotEmpty(t, link, sms["text"])

	// The parent opens the link and taps an answer.
	resp, err := http.Get(link)
	require.NoError(t, err)
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(page), "Tomorrow at 9.")
	assert.Contains(t, string(page), "Will attend")

	resp, err = http.PostForm(link, url.Values{"action": {"yes"}})
	require.NoError(t, err)
	page, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(page), "your answer was recorded: Will attend")

	answered := gw.call(http.MethodGet, "/_/api/notifications/"+notificationID+"/status", nil, http.StatusOK)
	assert.Equal(t, map[string]any{"sms": "sent"}, answered["channels"])

	// Removing the number.
	updated := gw.call(http.MethodPatch, "/_/api/users/"+parentID, map[string]any{"phone": ""}, http.StatusOK)
	assert.Nil(t, updated["phone"])
}

func TestPhoneNotifications_WhatsAppThroughTheAPI(t *testing.T) {
	meta := newRecordingAPI(t, `{"messaging_product":"whatsapp","messages":[{"id":"wamid.1"}]}`)
	gw := newPhoneGateway(t, config.NotificationConfig{WhatsApp: config.WhatsAppNotificationConfig{
		Enabled: true, PhoneNumberID: "pn", AccessToken: "token", APIURL: meta.URL,
		Template: config.WhatsAppTemplateConfig{Name: "taronja_notification", Language: "es", LinkButton: true},
	}})

	parent := gw.call(http.MethodPost, "/_/api/users", map[string]any{"username": "parent-2", "email": "parent2@example.com", "phone": "+34600999888"}, http.StatusCreated)
	parentID := parent["id"].(string)
	gw.call(http.MethodPost, "/_/api/notifications", map[string]any{
		"userIds": []string{parentID}, "type": "info", "title": "Closed", "body": "No school on Friday.",
		"channels": []string{"whatsapp"},
	}, http.StatusCreated)

	sent := meta.waitForOne(t)
	assert.Equal(t, "34600999888", sent["to"])
	template := sent["template"].(map[string]any)
	assert.Equal(t, "taronja_notification", template["name"])
	components := template["components"].([]any)
	require.Len(t, components, 2)
	token := components[1].(map[string]any)["parameters"].([]any)[0].(map[string]any)["text"].(string)

	resp, err := http.Get(gw.server.URL + "/_/n/" + token)
	require.NoError(t, err)
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(page), "No school on Friday.")
	assert.NotContains(t, string(page), `name="action"`, "no actions, no answer buttons")
}
