package billing

import (
	"context"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func brandEnv(t *testing.T) *testEnv {
	t.Helper()
	e := newEnv(t)
	e.svc.email = config.BillingEmailConfig{BrandName: "Taronja Global", LogoURL: "https://t.test/global.png"}
	e.svc.brands = map[string]config.BillingBrandConfig{
		"academia": {Name: "Academia", ButtonColor: "#112233", BackgroundColor: "#fafafa", EmailSubject: "Tu factura {number} de Academia"},
		"megabox":  {Name: "Megabox", LogoURL: "https://m.test/logo.png", EmailSubject: "Gracias por tu compra"},
	}
	return e
}

func TestBrandFor_FallsBackToTheGlobalEmailBrand(t *testing.T) {
	e := brandEnv(t)
	assert.Equal(t, Brand{Name: "Academia", LogoURL: "https://t.test/global.png", ButtonColor: "#112233", BackgroundColor: "#fafafa", EmailSubject: "Tu factura {number} de Academia"}, e.svc.BrandFor("academia"))
	assert.Equal(t, "https://m.test/logo.png", e.svc.BrandFor("megabox").LogoURL)
	assert.Equal(t, Brand{Name: "Taronja Global", LogoURL: "https://t.test/global.png"}, e.svc.BrandFor("unknown"))
	assert.Equal(t, e.svc.BrandFor("unknown"), e.svc.BrandFor(""))
}

func TestStartCheckout_CarriesTheBrandOfTheGroup(t *testing.T) {
	e := brandEnv(t)
	orgID := e.org(t, "Escuela", "u1")
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	_, _, err = e.svc.StartCheckout(context.Background(), CheckoutRequest{PlanKey: "academia-monthly", OrgID: orgID, Payer: Payer{UserID: "u1", Email: "e@x.es"}})
	require.NoError(t, err)
	assert.Equal(t, "Academia", e.stripe.Sessions[0].Brand.Name)
	assert.Equal(t, "#112233", e.stripe.Sessions[0].Brand.ButtonColor)
}

func TestInvoiceEmail_UsesTheBrandOfTheDocumentsPlanGroup(t *testing.T) {
	e := brandEnv(t)
	mailer := &fakeMailer{}
	e.svc.mailer = mailer
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "b12345674")
	sub := &db.BillingSubscription{Subject: "org:o1", PlanKey: "academia-monthly", Status: db.SubscriptionActive}
	require.NoError(t, e.repo.CreateSubscription(sub))
	d := &db.BillingDocument{Subject: "org:o1", Kind: db.DocumentInvoice, StripeRef: "in_1", AmountCents: 2900, PaidAt: e.clock, SubscriptionID: &sub.ID}
	require.NoError(t, EnqueueDocument(e.repo, d, []DocumentLine{{Description: "Academia mensual", PlanKey: "academia-monthly", AmountCents: 2900}}, e.clock))

	e.process(t)
	require.Len(t, mailer.sent, 1)
	m := mailer.sent[0]
	assert.Equal(t, "Tu factura F/2026/00001 de Academia", m.Subject)
	assert.Contains(t, m.HTML, "https://t.test/global.png")
	assert.Contains(t, m.HTML, "Academia")
	assert.NotContains(t, m.HTML, "Taronja Global")
}

func TestBrandEmailSubject(t *testing.T) {
	assert.Equal(t, "Factura N1 · Marca", Brand{}.emailSubject("Factura", "N1", "Marca"))
	assert.Equal(t, "Hola N1", Brand{EmailSubject: "Hola {number}"}.emailSubject("Factura", "N1", "Marca"))
	assert.Equal(t, "Gracias · Abono N1", Brand{EmailSubject: "Gracias"}.emailSubject("Abono", "N1", "Marca"))
}
