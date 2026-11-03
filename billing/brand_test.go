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
		"academia": {Name: "Academia", StatementDescriptor: "ACADEMIA TARONJA", ButtonColor: "#112233", BackgroundColor: "#fafafa", EmailSubject: "Tu factura {number} de Academia"},
		"megabox":  {Name: "Megabox", LogoURL: "https://m.test/logo.png", EmailSubject: "Gracias por tu compra"},
	}
	return e
}

func TestBrandFor_FallsBackToTheGlobalEmailBrand(t *testing.T) {
	e := brandEnv(t)
	assert.Equal(t, Brand{Name: "Academia", LogoURL: "https://t.test/global.png", ButtonColor: "#112233", BackgroundColor: "#fafafa", EmailSubject: "Tu factura {number} de Academia", StatementDescriptor: "ACADEMIA TARONJA"}, e.svc.BrandFor("academia"))
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

func TestStatementDescriptor_SubscriptionPlansUseTheProduct(t *testing.T) {
	e := brandEnv(t)
	ctx := context.Background()
	plan, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	assert.Equal(t, "ACADEMIA TARONJA", e.stripe.ProductDescriptors[plan.StripeProductID])

	other := monthlyInput()
	other.Key, other.Group = "megabox-monthly", "megabox"
	p2, err := e.svc.CreatePlan(ctx, other)
	require.NoError(t, err)
	assert.Empty(t, e.stripe.ProductDescriptors[p2.StripeProductID], "a group without a descriptor keeps the account's")

	delete(e.stripe.ProductDescriptors, plan.StripeProductID)
	require.NoError(t, e.svc.SyncStatementDescriptors(ctx))
	assert.Equal(t, "ACADEMIA TARONJA", e.stripe.ProductDescriptors[plan.StripeProductID], "applied again at startup")
	assert.Empty(t, e.stripe.ProductDescriptors[p2.StripeProductID])
}

func TestStatementDescriptor_OneTimePaymentsUseTheSuffix(t *testing.T) {
	e := brandEnv(t)
	ctx := context.Background()
	orgID := e.org(t, "Escuela", "u1")
	payer := Payer{UserID: "u1", Email: "e@x.es"}

	once := monthlyInput()
	once.Key, once.Kind, once.Interval, once.TrialDays = "academia-pack", db.PlanKindOneTime, "", 0
	_, err := e.svc.CreatePlan(ctx, once)
	require.NoError(t, err)
	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: "academia-pack", OrgID: orgID, Payer: payer})
	require.NoError(t, err)
	assert.Equal(t, "payment", e.stripe.Sessions[0].Mode)
	assert.Equal(t, "ACADEMIA TARONJA", e.stripe.Sessions[0].StatementDescriptorSuffix)

	_, err = e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	_, _, err = e.svc.StartCheckout(ctx, CheckoutRequest{PlanKey: "academia-monthly", OrgID: orgID, Payer: payer})
	require.NoError(t, err)
	assert.Equal(t, "subscription", e.stripe.Sessions[1].Mode)
	assert.Empty(t, e.stripe.Sessions[1].StatementDescriptorSuffix, "Stripe takes none for subscriptions")
}
