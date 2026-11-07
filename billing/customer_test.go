package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cuDetails(kind, taxID string) CustomerDetails {
	return CustomerDetails{
		Kind: kind, Name: "Escuela SL", TaxID: taxID, Phone: "+34600000000",
		Address: Address{Line1: "C/ Mayor 1", PostalCode: "46001", City: "Valencia", State: "Valencia", Country: "es"},
	}
}

// saveEnv has an owner of org o1 and no customer saved yet.
func saveEnv(t *testing.T) (*testEnv, string, Payer) {
	t.Helper()
	e := newEnv(t)
	orgID := e.org(t, "Escuela", "u1")
	return e, string(OrgSubject(orgID)), Payer{UserID: "u1", Email: "e@x.es"}
}

func TestSaveCustomerDetails_Company(t *testing.T) {
	e, subject, payer := saveEnv(t)
	c, err := e.svc.SaveCustomerDetails(context.Background(), subject, payer, cuDetails(KindCompany, "b-12345674"))
	require.NoError(t, err)
	assert.Equal(t, KindCompany, c.Kind)
	assert.Equal(t, "B12345674", c.TaxID)
	assert.Equal(t, "ES", c.Country)

	saved, err := e.repo.GetCustomer(subject)
	require.NoError(t, err)
	assert.Equal(t, "+34600000000", saved.Phone)
	assert.Equal(t, "Valencia", saved.City)

	require.Len(t, e.stripe.CustomerUpdates, 1)
	u := e.stripe.CustomerUpdates[0]
	assert.Equal(t, saved.StripeCustomerID, u.CustomerID)
	assert.Equal(t, "Escuela SL", u.Name)
	assert.Equal(t, "B12345674", u.TaxID)
	assert.Equal(t, "ES", u.Address.Country)

	p := e.odoo.Partners["tg:"+subject]
	assert.True(t, p.IsCompany)
	assert.Equal(t, "ESB12345674", p.VAT)
}

func TestSaveCustomerDetails_IndividualWithoutTaxID(t *testing.T) {
	e, subject, payer := saveEnv(t)
	c, err := e.svc.SaveCustomerDetails(context.Background(), subject, payer, cuDetails(KindIndividual, ""))
	require.NoError(t, err)
	assert.Equal(t, KindIndividual, c.Kind)
	assert.Empty(t, c.TaxID)
	assert.Empty(t, e.stripe.CustomerUpdates[0].TaxID)
	assert.False(t, e.odoo.Partners["tg:"+subject].IsCompany)
}

func TestSaveCustomerDetails_Invalid(t *testing.T) {
	e, subject, payer := saveEnv(t)
	mod := func(f func(*CustomerDetails)) CustomerDetails {
		d := cuDetails(KindCompany, "B12345674")
		f(&d)
		return d
	}
	for name, d := range map[string]CustomerDetails{
		"bad kind":         mod(func(d *CustomerDetails) { d.Kind = "x" }),
		"no name":          mod(func(d *CustomerDetails) { d.Name = " " }),
		"no address":       mod(func(d *CustomerDetails) { d.Address.Line1 = "" }),
		"no postal code":   mod(func(d *CustomerDetails) { d.Address.PostalCode = "" }),
		"no city":          mod(func(d *CustomerDetails) { d.Address.City = "" }),
		"bad country":      mod(func(d *CustomerDetails) { d.Address.Country = "ESP" }),
		"company no taxid": mod(func(d *CustomerDetails) { d.TaxID = "" }),
		"bad CIF":          mod(func(d *CustomerDetails) { d.TaxID = "B12345675" }),
	} {
		_, err := e.svc.SaveCustomerDetails(context.Background(), subject, payer, d)
		assert.ErrorIs(t, err, ErrInvalid, name)
	}
	assert.Empty(t, e.stripe.CustomerUpdates)
}

func TestSaveCustomerDetails_Forbidden(t *testing.T) {
	e, subject, _ := saveEnv(t)
	_, err := e.svc.SaveCustomerDetails(context.Background(), subject, Payer{UserID: "intruder"}, cuDetails(KindCompany, "B12345674"))
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.GetCustomerDetails(subject, "intruder")
	assert.ErrorIs(t, err, ErrForbidden)
	assert.Empty(t, e.stripe.CustomerUpdates)
}

func TestSaveCustomerDetails_TaxIDIsImmutable(t *testing.T) {
	e, subject, payer := saveEnv(t)
	ctx := context.Background()
	_, err := e.svc.SaveCustomerDetails(ctx, subject, payer, cuDetails(KindCompany, "B12345674"))
	require.NoError(t, err)

	_, err = e.svc.SaveCustomerDetails(ctx, subject, payer, cuDetails(KindCompany, "A58818501"))
	assert.ErrorIs(t, err, ErrConflict)

	d := cuDetails(KindCompany, "B 12345674")
	d.Name = "Escuela Nueva SL"
	c, err := e.svc.SaveCustomerDetails(ctx, subject, payer, d)
	require.NoError(t, err, "the same tax id again is fine")
	assert.Equal(t, "Escuela Nueva SL", c.Name)

	d.TaxID = ""
	c, err = e.svc.SaveCustomerDetails(ctx, subject, payer, d)
	require.NoError(t, err, "omitting it keeps the saved one")
	assert.Equal(t, "B12345674", c.TaxID)

	require.Len(t, e.stripe.CustomerUpdates, 3)
	assert.Equal(t, "B12345674", e.stripe.CustomerUpdates[0].TaxID)
	assert.Empty(t, e.stripe.CustomerUpdates[1].TaxID, "Stripe already has it")
	assert.Empty(t, e.stripe.CustomerUpdates[2].TaxID)
}

func TestSaveCustomerDetails_OdooDownDoesNotFail(t *testing.T) {
	e, subject, payer := saveEnv(t)
	e.odoo.FailPartner = errors.New("odoo down")
	c, err := e.svc.SaveCustomerDetails(context.Background(), subject, payer, cuDetails(KindCompany, "B12345674"))
	require.NoError(t, err)
	assert.Equal(t, "B12345674", c.TaxID)
	saved, err := e.repo.GetCustomer(subject)
	require.NoError(t, err)
	assert.Equal(t, KindCompany, saved.Kind)
}

func TestGetCustomerDetails(t *testing.T) {
	e, subject, payer := saveEnv(t)
	c, err := e.svc.GetCustomerDetails(subject, "u1")
	require.NoError(t, err)
	assert.Empty(t, c.Kind)
	assert.Empty(t, c.Name)

	_, err = e.svc.SaveCustomerDetails(context.Background(), subject, payer, cuDetails(KindIndividual, "12345678Z"))
	require.NoError(t, err)
	c, err = e.svc.GetCustomerDetails(subject, "u1")
	require.NoError(t, err)
	assert.Equal(t, KindIndividual, c.Kind)
	assert.Equal(t, "12345678Z", c.TaxID)
}

func TestStartCheckout_UsesSavedCustomer(t *testing.T) {
	e, subject, payer := saveEnv(t)
	ctx := context.Background()
	_, err := e.svc.CreatePlan(ctx, monthlyInput())
	require.NoError(t, err)
	orgID := subject[len("org:"):]

	req := CheckoutRequest{PlanKey: "academia-monthly", OrgID: orgID, Payer: payer}
	_, _, err = e.svc.StartCheckout(ctx, req)
	require.NoError(t, err)
	require.Len(t, e.stripe.Sessions, 1)
	assert.True(t, e.stripe.Sessions[0].AskCustomerType, "nothing saved yet: Checkout asks")

	saved, err := e.svc.SaveCustomerDetails(ctx, subject, payer, cuDetails(KindCompany, "B12345674"))
	require.NoError(t, err)
	_, _, err = e.svc.StartCheckout(ctx, req)
	require.NoError(t, err)
	require.Len(t, e.stripe.Sessions, 2)
	assert.False(t, e.stripe.Sessions[1].AskCustomerType)
	assert.Equal(t, saved.StripeCustomerID, e.stripe.Sessions[1].CustomerID)
}

type fakeTaxChecker struct {
	valid bool
	err   error
	calls int
}

func (f *fakeTaxChecker) Check(context.Context, string, string) (bool, error) {
	f.calls++
	return f.valid, f.err
}

func TestSaveCustomerDetails_ForeignTaxIDIsChecked(t *testing.T) {
	foreign := func(taxID string) CustomerDetails {
		d := cuDetails(KindCompany, taxID)
		d.Address.Country = "FR"
		return d
	}
	ctx := context.Background()

	e, subject, payer := saveEnv(t)
	checker := &fakeTaxChecker{valid: false}
	e.svc.taxChecker = checker
	_, err := e.svc.SaveCustomerDetails(ctx, subject, payer, foreign("FR12345678901"))
	assert.ErrorIs(t, err, ErrInvalid)
	assert.Equal(t, 1, checker.calls)

	// A checker that cannot tell never blocks a customer.
	checker.err = errors.New("VIES down")
	c, err := e.svc.SaveCustomerDetails(ctx, subject, payer, foreign("FR12345678901"))
	require.NoError(t, err)
	assert.Equal(t, "FR12345678901", c.TaxID)

	// Spanish ids are checked offline, never asked to VIES.
	e2, subject2, payer2 := saveEnv(t)
	checker2 := &fakeTaxChecker{valid: false}
	e2.svc.taxChecker = checker2
	_, err = e2.svc.SaveCustomerDetails(ctx, subject2, payer2, cuDetails(KindCompany, "B12345674"))
	require.NoError(t, err)
	assert.Zero(t, checker2.calls)
}
