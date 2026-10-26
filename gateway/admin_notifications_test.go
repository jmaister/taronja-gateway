package gateway

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminNotificationList(t *testing.T) {
	bulkgate := newRecordingAPI(t, `{"data":{"status":"accepted","sms_id":"sms-1"}}`)
	gw := newPhoneGateway(t, config.NotificationConfig{SMS: config.SMSNotificationConfig{
		Enabled: true, BulkGate: config.BulkGateConfig{ApplicationID: "app", ApplicationToken: "tok", APIURL: bulkgate.URL},
	}})

	parent := gw.call(http.MethodPost, "/_/api/users", map[string]any{"username": "lucia", "email": "lucia@parents.example", "phone": "+34600111222"}, http.StatusCreated)
	other := gw.call(http.MethodPost, "/_/api/users", map[string]any{"username": "marc", "email": "marc@parents.example"}, http.StatusCreated)
	send := func(userID, kind, title string, channels ...string) {
		body := map[string]any{"userIds": []string{userID}, "type": kind, "title": title, "body": "Body: " + title}
		if len(channels) > 0 {
			body["channels"] = channels
		}
		gw.call(http.MethodPost, "/_/api/notifications", body, http.StatusCreated)
	}
	send(parent["id"].(string), "trip", "Excursion", "sms")  // sent
	send(other["id"].(string), "trip", "Excursion", "sms")   // skipped: no phone
	send(other["id"].(string), "news", "Newsletter")         // no channels named: every configured one (sms), skipped: no phone
	send(parent["id"].(string), "fees", "Fees due", "email") // skipped: email is not configured

	list := func(query url.Values) map[string]any {
		t.Helper()
		return gw.call(http.MethodGet, "/_/api/admin/notifications?"+query.Encode(), nil, http.StatusOK)
	}
	bulkgate.waitForOne(t)
	// Deliveries run in the background; wait until all four have settled.
	deadline := time.Now().Add(5 * time.Second)
	for list(url.Values{"status": {"pending"}})["total"].(float64) > 0 || list(url.Values{"status": {"none"}})["total"].(float64) > 0 {
		require.True(t, time.Now().Before(deadline), "deliveries did not settle")
		time.Sleep(20 * time.Millisecond)
	}
	items := func(page map[string]any) []map[string]any {
		var out []map[string]any
		for _, item := range page["items"].([]any) {
			out = append(out, item.(map[string]any))
		}
		return out
	}

	all := list(url.Values{})
	assert.EqualValues(t, 4, all["total"])
	assert.EqualValues(t, 1, all["page"])
	assert.EqualValues(t, 50, all["limit"])
	newest := items(all)[0]
	assert.Equal(t, "Fees due", newest["notification"].(map[string]any)["title"])
	assert.Equal(t, "lucia", newest["username"])

	sent := list(url.Values{"status": {"sent"}})
	require.EqualValues(t, 1, sent["total"])
	row := items(sent)[0]
	assert.Equal(t, "lucia@parents.example", row["email"])
	assert.Equal(t, map[string]any{"sms": "sent"}, row["channels"])
	assert.Equal(t, "sent", row["status"])

	assert.EqualValues(t, 3, list(url.Values{"status": {"skipped"}})["total"])
	assert.EqualValues(t, 2, list(url.Values{"type": {"trip"}})["total"])
	assert.EqualValues(t, 2, list(url.Values{"user": {"MARC"}})["total"])
	assert.EqualValues(t, 1, list(url.Values{"q": {"newsletter"}})["total"])
	assert.EqualValues(t, 3, list(url.Values{"channel": {"sms"}})["total"])
	assert.EqualValues(t, 0, list(url.Values{"end_date": {"2000-01-01T00:00:00Z"}})["total"])
	assert.EqualValues(t, 4, list(url.Values{"start_date": {time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}})["total"])

	page2 := list(url.Values{"limit": {"3"}, "page": {"2"}})
	assert.EqualValues(t, 4, page2["total"])
	require.Len(t, items(page2), 1, "the last page holds the rest")
	assert.Equal(t, "Excursion", items(page2)[0]["notification"].(map[string]any)["title"])

	gw.call(http.MethodGet, "/_/api/admin/notifications?status=lost", nil, http.StatusBadRequest)
	gw.call(http.MethodGet, "/_/api/admin/notifications?limit=500", nil, http.StatusBadRequest)
	gw.call(http.MethodGet, "/_/api/admin/notifications?start_date=2026-02-01T00:00:00Z&end_date=2026-01-01T00:00:00Z", nil, http.StatusBadRequest)

	types := gw.call(http.MethodGet, "/_/api/admin/notifications/types", nil, http.StatusOK)
	assert.Equal(t, []any{"fees", "news", "trip"}, types["types"])
}

func TestAdminNotificationList_AdminsOnly(t *testing.T) {
	gw := newPhoneGateway(t, config.NotificationConfig{})
	d := gw.deps
	user := &db.User{Username: "teacher", Email: "teacher@school.example", EmailConfirmed: true}
	require.NoError(t, d.UserRepo.CreateUser(user))
	sess, err := d.SessionStore.NewSession(nil, user, "basic", time.Hour)
	require.NoError(t, err)

	for _, path := range []string{"/_/api/admin/notifications", "/_/api/admin/notifications/types"} {
		req, err := http.NewRequest(http.MethodGet, gw.server.URL+path, nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: sess.Token})
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, path)
	}
}
