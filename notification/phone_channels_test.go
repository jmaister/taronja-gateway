package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePhone(t *testing.T) {
	phone, err := NormalizePhone(" +34 600-12.34 (56) ")
	require.NoError(t, err)
	assert.Equal(t, "+34600123456", phone)
	for _, bad := range []string{"", "600123456", "+0600123456", "+34abc", "+1234", "+1234567890123456"} {
		_, err := NormalizePhone(bad)
		assert.ErrorIs(t, err, ErrInvalidPhone, bad)
	}
}

func TestSMSSegments(t *testing.T) {
	assert.Equal(t, 1, smsSegments(strings.Repeat("a", 160)))
	assert.Equal(t, 2, smsSegments(strings.Repeat("a", 161)))
	assert.Equal(t, 2, smsSegments(strings.Repeat("a", 306)))
	assert.Equal(t, 3, smsSegments(strings.Repeat("a", 307)))
	assert.Equal(t, 2, smsSegments(strings.Repeat("€", 81)), "extended characters take two positions")
	assert.Equal(t, 1, smsSegments(strings.Repeat("á", 70)), "á is not GSM-7: UCS-2")
	assert.Equal(t, 2, smsSegments(strings.Repeat("á", 71)))
	assert.True(t, isGSM("Reunion de padres ñ é ü"))
	assert.False(t, isGSM("Reunión"))
}

func TestComposeSMS(t *testing.T) {
	text, unicode := composeSMS("Excursión", "Mañana salimos a las 9.", "https://gw.example/_/n/abc", 2)
	assert.Equal(t, "Excursión\nMañana salimos a las 9.\nhttps://gw.example/_/n/abc", text)
	assert.True(t, unicode)

	long := strings.Repeat("palabra ", 100)
	link := "https://gw.example/_/n/tok"
	text, unicode = composeSMS("Aviso", long, link, 2)
	assert.False(t, unicode)
	assert.LessOrEqual(t, smsSegments(text), 2)
	assert.True(t, strings.HasSuffix(text, "...\n"+link), "the body is shortened, the link kept: %q", text)
	assert.True(t, strings.HasPrefix(text, "Aviso\npalabra"))

	text, unicode = composeSMS("Aviso", strings.Repeat("canción ", 100), link, 1)
	assert.True(t, unicode)
	assert.Equal(t, 1, smsSegments(text))
	assert.Contains(t, text, "…\n"+link)

	text, _ = composeSMS(strings.Repeat("T", 300), "body", "", 1)
	assert.Equal(t, 1, smsSegments(text), "a very long title is shortened too")
}

func TestTemplateText(t *testing.T) {
	assert.Equal(t, "Line one Line two tab spaced", templateText("Line one\nLine two\ttab      spaced", 100))
	assert.Equal(t, "-", templateText("  \n ", 100), "Meta rejects empty variables")
	assert.Equal(t, "abcd…", templateText("abcdefgh", 5))
}

// fakeAPI records the JSON bodies it receives and answers with the next
// queued response (or the last one).
type fakeAPI struct {
	*httptest.Server
	mu        sync.Mutex
	requests  []map[string]any
	paths     []string
	auth      []string
	responses []fakeResponse
}

type fakeResponse struct {
	status int
	body   string
}

func newFakeAPI(t *testing.T, responses ...fakeResponse) *fakeAPI {
	f := &fakeAPI{responses: responses}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, body)
		f.paths = append(f.paths, r.URL.Path)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		resp := f.responses[0]
		if len(f.responses) > 1 {
			f.responses = f.responses[1:]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		w.Write([]byte(resp.body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAPI) last(t *testing.T) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.NotEmpty(t, f.requests, "the API was not called")
	return f.requests[len(f.requests)-1]
}

func (f *fakeAPI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

const bulkGateAccepted = `{"data":{"status":"accepted","sms_id":"sms-123","part_id":["sms-123_1"],"number":"34600123456"}}`

// phoneFixture is a service with phone channels, one user with a phone number
// and one without.
type phoneFixture struct {
	service   *Service
	repo      repositories.NotificationRepository
	phones    repositories.UserPhoneRepository
	withPhone *db.User
	noPhone   *db.User
}

func newPhoneFixture(t *testing.T, cfg config.NotificationConfig) *phoneFixture {
	t.Helper()
	service, repo, userRepo := newTestService(t, cfg)
	phones := repositories.NewUserPhoneRepositoryDB(db.GetConnection())
	service.ConfigurePhoneChannels(phones, "https://gw.example.com")
	withPhone := &db.User{Username: "parent", Email: "parent@example.com"}
	require.NoError(t, userRepo.CreateUser(withPhone))
	require.NoError(t, phones.Set(withPhone.ID, "+34600123456", nil))
	noPhone := &db.User{Username: "nophone", Email: "nophone@example.com"}
	require.NoError(t, userRepo.CreateUser(noPhone))
	return &phoneFixture{service: service, repo: repo, phones: phones, withPhone: withPhone, noPhone: noPhone}
}

func (f *phoneFixture) deliveries(t *testing.T, n *db.Notification) []*db.NotificationDelivery {
	t.Helper()
	deliveries, err := f.repo.ListDeliveries(n.ID)
	require.NoError(t, err)
	return deliveries
}

func smsConfig(url string) config.NotificationConfig {
	return config.NotificationConfig{SMS: config.SMSNotificationConfig{
		Enabled:  true,
		BulkGate: config.BulkGateConfig{ApplicationID: "app-1", ApplicationToken: "tok-1", SenderID: "gText", SenderIDValue: "Academia", APIURL: url},
	}}
}

func TestSMS_DeliversThroughBulkGateWithAPageLink(t *testing.T) {
	bulkgate := newFakeAPI(t, fakeResponse{200, bulkGateAccepted})
	f := newPhoneFixture(t, smsConfig(bulkgate.URL))

	n := createOne(t, f.service, CreateInput{
		UserIDs: []string{f.withPhone.ID}, Type: "trip", Title: "Excursión", Body: "Mañana a las 9.",
		Actions:  []Action{{ID: "ok", Label: "Asistirá"}, {ID: "no", Label: "No asistirá"}},
		Channels: []string{db.NotificationChannelSMS},
	})

	sent := bulkgate.last(t)
	assert.Equal(t, "app-1", sent["application_id"])
	assert.Equal(t, "tok-1", sent["application_token"])
	assert.Equal(t, "34600123456", sent["number"], "BulkGate gets the number without +")
	assert.Equal(t, true, sent["unicode"], "ó and ñ need UCS-2")
	assert.Equal(t, "gText", sent["sender_id"])
	assert.Equal(t, "Academia", sent["sender_id_value"])
	text := sent["text"].(string)
	assert.True(t, strings.HasPrefix(text, "Excursión\nMañana a las 9.\nhttps://gw.example.com/_/n/"), text)

	deliveries := f.deliveries(t, n)
	require.Len(t, deliveries, 2)
	assert.Equal(t, db.NotificationDeliveryStatusSent, deliveries[0].Status)
	assert.Equal(t, "sms-123", deliveries[0].ExternalRef)

	// The page link shows the notification and records the answer as SMS.
	token := text[strings.LastIndex(text, "/")+1:]
	page, actions, err := f.service.NotificationPage(token)
	require.NoError(t, err)
	assert.Equal(t, n.ID, page.ID)
	assert.Len(t, actions, 2)
	label, err := f.service.RespondViaLink(context.Background(), token, "ok")
	require.NoError(t, err)
	assert.Equal(t, "Asistirá", label)
	answered, err := f.repo.GetNotification(n.ID)
	require.NoError(t, err)
	assert.Equal(t, db.NotificationChannelSMS, *answered.RespondedVia)
	_, err = f.service.RespondViaLink(context.Background(), token, "no")
	assert.ErrorIs(t, err, ErrAlreadyResponded)

	_, _, err = f.service.NotificationPage("not-a-token")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSMS_NoLinkWithoutActionsOrURL(t *testing.T) {
	bulkgate := newFakeAPI(t, fakeResponse{200, bulkGateAccepted})
	f := newPhoneFixture(t, smsConfig(bulkgate.URL))
	createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "info", Title: "Hello", Body: "No school on Friday.", Channels: []string{"sms"}})
	assert.Equal(t, "Hello\nNo school on Friday.", bulkgate.last(t)["text"])
	assert.Equal(t, false, bulkgate.last(t)["unicode"])
}

func TestSMS_PermanentErrorsAreNotRetried(t *testing.T) {
	bulkgate := newFakeAPI(t, fakeResponse{400, `{"type":"invalid_phone_number","code":400,"error":"Invalid phone number","detail":null}`})
	f := newPhoneFixture(t, smsConfig(bulkgate.URL))
	n := createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "t", Title: "T", Body: "B", Channels: []string{"sms"}})

	failed := f.deliveries(t, n)[0]
	assert.Equal(t, db.NotificationDeliveryStatusFailed, failed.Status)
	assert.Contains(t, failed.Error, "invalid_phone_number")
	assert.Nil(t, failed.NextRetryAt, "an invalid number is not paid for again")
}

func TestSMS_ServerErrorsAreRetried(t *testing.T) {
	bulkgate := newFakeAPI(t, fakeResponse{503, `{"type":"unavailable","code":503,"error":"Service unavailable"}`}, fakeResponse{200, bulkGateAccepted})
	f := newPhoneFixture(t, smsConfig(bulkgate.URL))
	n := createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "t", Title: "T", Body: "B", Channels: []string{"sms"}})

	failed := f.deliveries(t, n)[0]
	assert.Equal(t, db.NotificationDeliveryStatusFailed, failed.Status)
	require.NotNil(t, failed.NextRetryAt)

	backdateNextRetry(t, failed.ID)
	attempted, err := f.service.RetryFailedDeliveries(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, attempted)
	assert.Equal(t, db.NotificationDeliveryStatusSent, f.deliveries(t, n)[0].Status)
}

func TestSMS_SkippedWithoutAPhoneNumber(t *testing.T) {
	bulkgate := newFakeAPI(t, fakeResponse{200, bulkGateAccepted})
	f := newPhoneFixture(t, smsConfig(bulkgate.URL))
	n := createOne(t, f.service, CreateInput{UserIDs: []string{f.noPhone.ID}, Type: "t", Title: "T", Body: "B", Channels: []string{"sms"}})
	delivery := f.deliveries(t, n)[0]
	assert.Equal(t, db.NotificationDeliveryStatusSkipped, delivery.Status)
	assert.Contains(t, delivery.Error, "no phone number")
	assert.Equal(t, 0, bulkgate.count())
}

func TestSMS_RateLimitDefersToTheRetryWorker(t *testing.T) {
	bulkgate := newFakeAPI(t, fakeResponse{200, bulkGateAccepted})
	cfg := smsConfig(bulkgate.URL)
	cfg.SMS.MaxPerMinute = 1
	f := newPhoneFixture(t, cfg)
	first := createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "t", Title: "1", Body: "B", Channels: []string{"sms"}})
	second := createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "t", Title: "2", Body: "B", Channels: []string{"sms"}})

	assert.Equal(t, db.NotificationDeliveryStatusSent, f.deliveries(t, first)[0].Status)
	limited := f.deliveries(t, second)[0]
	assert.Equal(t, db.NotificationDeliveryStatusFailed, limited.Status)
	assert.Contains(t, limited.Error, "rate limit")
	assert.NotNil(t, limited.NextRetryAt, "sent by a later retry")
	assert.Equal(t, 1, bulkgate.count())
}

func TestPhoneChannelStatus(t *testing.T) {
	bulkgate := newFakeAPI(t, fakeResponse{200, bulkGateAccepted})
	f := newPhoneFixture(t, smsConfig(bulkgate.URL))
	statusOf := func(userID string) string {
		statuses, err := f.service.ChannelStatuses(userID)
		require.NoError(t, err)
		for _, s := range statuses {
			if s.Channel == "sms" {
				return s.Status
			}
		}
		return ""
	}
	assert.Equal(t, ChannelStatusConnected, statusOf(f.withPhone.ID))
	assert.Equal(t, ChannelStatusNotConnected, statusOf(f.noPhone.ID))
}

func whatsAppConfig(url string, linkButton bool) config.NotificationConfig {
	return config.NotificationConfig{WhatsApp: config.WhatsAppNotificationConfig{
		Enabled: true, PhoneNumberID: "pn-1", AccessToken: "wa-token", APIURL: url,
		Template: config.WhatsAppTemplateConfig{Name: "taronja_notification", Language: "es", LinkButton: linkButton},
	}}
}

const whatsAppAccepted = `{"messaging_product":"whatsapp","contacts":[{"input":"34600123456","wa_id":"34600123456"}],"messages":[{"id":"wamid.ABC"}]}`

func TestWhatsApp_SendsTheApprovedTemplate(t *testing.T) {
	meta := newFakeAPI(t, fakeResponse{200, whatsAppAccepted})
	f := newPhoneFixture(t, whatsAppConfig(meta.URL, true))

	n := createOne(t, f.service, CreateInput{
		UserIDs: []string{f.withPhone.ID}, Type: "trip", Title: "Excursión",
		Body:     "Mañana a las 9.\nTraed\tagua.",
		Actions:  []Action{{ID: "ok", Label: "Asistirá"}},
		Channels: []string{db.NotificationChannelWhatsApp},
	})

	assert.Equal(t, "/v23.0/pn-1/messages", meta.paths[0])
	assert.Equal(t, "Bearer wa-token", meta.auth[0])
	sent := meta.last(t)
	assert.Equal(t, "whatsapp", sent["messaging_product"])
	assert.Equal(t, "34600123456", sent["to"])
	assert.Equal(t, "template", sent["type"])
	template := sent["template"].(map[string]any)
	assert.Equal(t, "taronja_notification", template["name"])
	assert.Equal(t, map[string]any{"code": "es"}, template["language"])
	components := template["components"].([]any)
	require.Len(t, components, 2)
	body := components[0].(map[string]any)["parameters"].([]any)
	assert.Equal(t, "Excursión", body[0].(map[string]any)["text"])
	assert.Equal(t, "Mañana a las 9. Traed agua.", body[1].(map[string]any)["text"], "no newlines or tabs in template variables")
	button := components[1].(map[string]any)
	assert.Equal(t, "url", button["sub_type"])
	token := button["parameters"].([]any)[0].(map[string]any)["text"].(string)

	page, _, err := f.service.NotificationPage(token)
	require.NoError(t, err, "the button's URL suffix is the page token")
	assert.Equal(t, n.ID, page.ID)
	assert.Equal(t, "wamid.ABC", f.deliveries(t, n)[0].ExternalRef)
}

func TestWhatsApp_WithoutLinkButton(t *testing.T) {
	meta := newFakeAPI(t, fakeResponse{200, whatsAppAccepted})
	f := newPhoneFixture(t, whatsAppConfig(meta.URL, false))
	createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "t", Title: "T", Body: "B", Actions: []Action{{ID: "a", Label: "A"}}, Channels: []string{"whatsapp"}})
	components := meta.last(t)["template"].(map[string]any)["components"].([]any)
	assert.Len(t, components, 1, "only the body variables")
}

func TestWhatsApp_ErrorClassification(t *testing.T) {
	notOnWhatsApp := `{"error":{"message":"(#131026) Message undeliverable","type":"OAuthException","code":131026,"error_data":{"messaging_product":"whatsapp","details":"Receiver is incapable of receiving this message"},"fbtrace_id":"x"}}`
	throttled := `{"error":{"message":"(#130429) Rate limit hit","type":"OAuthException","code":130429,"error_data":{"messaging_product":"whatsapp","details":"Cloud API message throughput has been reached."},"fbtrace_id":"y"}}`

	meta := newFakeAPI(t, fakeResponse{400, notOnWhatsApp}, fakeResponse{400, throttled})
	f := newPhoneFixture(t, whatsAppConfig(meta.URL, false))

	n := createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "t", Title: "T", Body: "B", Channels: []string{"whatsapp"}})
	failed := f.deliveries(t, n)[0]
	assert.Contains(t, failed.Error, "131026")
	assert.Nil(t, failed.NextRetryAt, "a number without WhatsApp is not retried")

	n = createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "t", Title: "T", Body: "B", Channels: []string{"whatsapp"}})
	failed = f.deliveries(t, n)[0]
	assert.Contains(t, failed.Error, "130429")
	assert.NotNil(t, failed.NextRetryAt, "throttling is retried")
}

func TestWhatsApp_BodyVariables(t *testing.T) {
	for _, tc := range []struct {
		variables      int
		wantComponents int
		wantFirst      string
	}{
		{1, 1, "Closed: No school on Friday."},
		{0, 0, ""},
	} {
		meta := newFakeAPI(t, fakeResponse{200, whatsAppAccepted})
		cfg := whatsAppConfig(meta.URL, false)
		variables := tc.variables
		cfg.WhatsApp.Template.BodyVariables = &variables
		f := newPhoneFixture(t, cfg)
		createOne(t, f.service, CreateInput{UserIDs: []string{f.withPhone.ID}, Type: "t", Title: "Closed", Body: "No school on Friday.", Channels: []string{"whatsapp"}})

		template := meta.last(t)["template"].(map[string]any)
		if tc.wantComponents == 0 {
			assert.NotContains(t, template, "components", "a fixed-text template like hello_world gets no parameters")
			continue
		}
		body := template["components"].([]any)[0].(map[string]any)["parameters"].([]any)
		require.Len(t, body, 1)
		assert.Equal(t, tc.wantFirst, body[0].(map[string]any)["text"])
	}
}
