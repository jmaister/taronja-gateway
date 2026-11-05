package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const subjectCU = "org:o1"

// customerEnv has a customer saved as Checkout left it, linked to cus_1.
func customerEnv(t *testing.T, taxID string) *testEnv {
	t.Helper()
	e := newEnv(t)
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, subjectCU, taxID)
	c, err := e.repo.GetCustomer(subjectCU)
	require.NoError(t, err)
	c.StripeCustomerID = "cus_1"
	require.NoError(t, e.repo.SaveCustomer(c))
	return e
}

func (e *testEnv) customerUpdated(t *testing.T, id string, at time.Time, cu CustomerEvent) {
	t.Helper()
	if cu.CustomerID == "" {
		cu.CustomerID = "cus_1"
	}
	e.event(t, Event{ID: id, Type: EventCustomerUpdated, Created: at, Customer: &cu})
}

func (e *testEnv) saved(t *testing.T) *db.BillingCustomer {
	t.Helper()
	c, err := e.repo.GetCustomer(subjectCU)
	require.NoError(t, err)
	return c
}

func TestCustomerUpdated_NameEmailAddress(t *testing.T) {
	e := customerEnv(t, "B12345674")
	e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{
		Name: "Escuela Nueva SL", Email: "nueva@x.es",
		Address: Address{Line1: "C/ Paz 2", PostalCode: "28001", City: "Madrid", State: "Madrid", Country: "ES"},
	})
	c := e.saved(t)
	assert.Equal(t, "Escuela Nueva SL", c.Name)
	assert.Equal(t, "nueva@x.es", c.Email)
	assert.Equal(t, "C/ Paz 2", c.AddressLine1)
	assert.Equal(t, "Madrid", c.City)
	assert.Equal(t, "B12345674", c.TaxID)
	assert.Equal(t, "cus_1", c.StripeCustomerID)
}

func TestCustomerUpdated_EmptyFieldsKeepSavedOnes(t *testing.T) {
	e := customerEnv(t, "B12345674")
	e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{Name: "Otro"})
	c := e.saved(t)
	assert.Equal(t, "Otro", c.Name)
	assert.Equal(t, "e@x.es", c.Email)
	assert.Equal(t, "Valencia", c.City)
}

func TestCustomerUpdated_TaxID(t *testing.T) {
	t.Run("set when none is saved", func(t *testing.T) {
		e := customerEnv(t, "")
		e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{TaxID: "B12345674"})
		assert.Equal(t, "B12345674", e.saved(t).TaxID)
	})
	t.Run("not overwritten by a different one", func(t *testing.T) {
		e := customerEnv(t, "B12345674")
		e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{Name: "Nuevo", TaxID: "A58818501"})
		c := e.saved(t)
		assert.Equal(t, "B12345674", c.TaxID)
		assert.Equal(t, "Nuevo", c.Name, "the rest of the event is applied")
	})
	t.Run("removed at Stripe keeps the saved one", func(t *testing.T) {
		e := customerEnv(t, "B12345674")
		e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{Name: "Nuevo", TaxID: ""})
		assert.Equal(t, "B12345674", e.saved(t).TaxID)
	})
}

func TestStripeTaxID_InvalidIsNotSaved(t *testing.T) {
	t.Run("from customer.updated", func(t *testing.T) {
		e := customerEnv(t, "")
		e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{Name: "Nuevo", TaxID: "B12345678"})
		c := e.saved(t)
		assert.Empty(t, c.TaxID)
		assert.Equal(t, "Nuevo", c.Name)
	})
	t.Run("from checkout.session.completed", func(t *testing.T) {
		e := customerEnv(t, "")
		checkout := e.checkout(t, subjectCU, "academia-monthly")
		e.event(t, Event{ID: "evt_done", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{
			CheckoutID: checkout, Mode: "subscription", Paid: true, CustomerID: "cus_1", TaxID: "12345678Z1",
		}})
		assert.Empty(t, e.saved(t).TaxID)
	})
	t.Run("a valid one is saved normalized", func(t *testing.T) {
		e := customerEnv(t, "")
		e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{TaxID: " b-12345674 "})
		assert.Equal(t, "B12345674", e.saved(t).TaxID)
	})
}

func TestCustomerUpdated_UnknownCustomerIgnored(t *testing.T) {
	e := customerEnv(t, "B12345674")
	e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{CustomerID: "cus_other", Name: "Intruso"})
	assert.Equal(t, "Escuela SL", e.saved(t).Name)
	_, err := e.repo.GetCustomerByStripeID("cus_other")
	assert.Error(t, err, "no customer is created")
}

func TestCustomerUpdated_DuplicateEventAppliedOnce(t *testing.T) {
	e := customerEnv(t, "B12345674")
	at := e.clock.Add(time.Minute)
	e.customerUpdated(t, "evt_1", at, CustomerEvent{Name: "Uno"})
	c := e.saved(t)
	c.Name = "Editado a mano"
	require.NoError(t, e.repo.SaveCustomer(c))
	e.customerUpdated(t, "evt_1", at, CustomerEvent{Name: "Uno"})
	assert.Equal(t, "Editado a mano", e.saved(t).Name, "the repeated event changes nothing")
}

func TestCustomerUpdated_OutOfOrder(t *testing.T) {
	e := customerEnv(t, "B12345674")
	t0 := e.clock
	e.customerUpdated(t, "evt_new", t0.Add(2*time.Minute), CustomerEvent{Name: "Segundo", Address: Address{Line1: "C/ Nueva 2", City: "Madrid", Country: "ES"}})
	e.customerUpdated(t, "evt_old", t0.Add(time.Minute), CustomerEvent{Name: "Primero", Address: Address{Line1: "C/ Vieja 1", City: "Sevilla", Country: "ES"}})
	c := e.saved(t)
	assert.Equal(t, "Segundo", c.Name)
	assert.Equal(t, "C/ Nueva 2", c.AddressLine1)
	assert.Equal(t, "Madrid", c.City)
}

func TestCustomerUpdated_NoChangesIsANoop(t *testing.T) {
	e := customerEnv(t, "B12345674")
	before := e.saved(t)
	e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{
		Name: before.Name, Email: before.Email, TaxID: before.TaxID,
		Address: Address{Line1: before.AddressLine1, PostalCode: before.PostalCode, City: before.City, State: before.State, Country: before.Country},
	})
	after := e.saved(t)
	assert.Equal(t, before.Name, after.Name)
	assert.Equal(t, before.Email, after.Email)
	assert.Equal(t, before.TaxID, after.TaxID)
	assert.Equal(t, before.AddressLine1, after.AddressLine1)
	assert.Equal(t, before.City, after.City)
}

func TestCustomerUpdated_NextInvoiceUsesNewData(t *testing.T) {
	e := customerEnv(t, "B12345674")
	first := e.invoice(t, subjectCU, "in_1")
	e.process(t)
	assert.Equal(t, "Escuela SL", e.odoo.Partners["tg:"+subjectCU].Name)
	issued := e.odoo.Moves["in_1"]

	e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{
		Name: "Escuela Nueva SL", Address: Address{Line1: "C/ Paz 2", PostalCode: "28001", City: "Madrid", State: "Madrid", Country: "ES"},
	})
	assert.Equal(t, "Escuela SL", e.odoo.Partners["tg:"+subjectCU].Name, "Odoo is updated with the next invoice")
	assert.Equal(t, issued, e.odoo.Moves["in_1"], "issued invoices stay as they are")
	assert.Equal(t, db.OdooStateDone, e.doc(t, first.ID).OdooState)

	e.invoice(t, subjectCU, "in_2")
	e.process(t)
	p := e.odoo.Partners["tg:"+subjectCU]
	assert.Equal(t, "Escuela Nueva SL", p.Name)
	assert.Equal(t, "Madrid", p.Address.City)
	assert.Equal(t, "C/ Paz 2", p.Address.Line1)
	assert.Equal(t, "ESB12345674", p.VAT)
	assert.Equal(t, e.odoo.PartnerIDs["tg:"+subjectCU], e.odoo.Moves["in_2"].PartnerID)
}

func TestCustomerUpdated_OdooDownThenRecovered(t *testing.T) {
	e := customerEnv(t, "B12345674")
	d := e.invoice(t, subjectCU, "in_1")
	e.odoo.FailIssue = errors.New("odoo is down")
	e.process(t)
	require.Equal(t, db.OdooStatePending, e.doc(t, d.ID).OdooState)

	e.customerUpdated(t, "evt_1", e.clock.Add(time.Minute), CustomerEvent{Name: "Escuela Nueva SL"})

	e.odoo.FailIssue = nil
	e.clock = e.clock.Add(time.Hour)
	assert.Equal(t, 1, e.process(t))
	assert.Equal(t, db.OdooStateDone, e.doc(t, d.ID).OdooState)
	assert.Equal(t, "Escuela Nueva SL", e.odoo.Partners["tg:"+subjectCU].Name)
}

func TestCheckoutCompleted_DoesNotRevertCustomerEditsOrNIF(t *testing.T) {
	e := customerEnv(t, "B12345674")
	e.customerUpdated(t, "evt_edit", e.clock.Add(time.Minute), CustomerEvent{Name: "Escuela Nueva SL"})

	checkout := e.checkout(t, subjectCU, "academia-monthly")
	e.event(t, Event{ID: "evt_done", Type: EventCheckoutCompleted, Created: e.clock.Add(2 * time.Minute), Checkout: &CheckoutEvent{
		CheckoutID: checkout, Mode: "subscription", Paid: true, CustomerID: "cus_1",
		Name: "Escuela Nueva SL", TaxID: "A58818501", Email: "e@x.es",
	}})
	c := e.saved(t)
	assert.Equal(t, "B12345674", c.TaxID, "a saved NIF is never replaced")
	assert.Equal(t, "Escuela Nueva SL", c.Name)
}

func TestCheckoutCompleted_SetsTaxIDWhenNoneSaved(t *testing.T) {
	e := customerEnv(t, "")
	checkout := e.checkout(t, subjectCU, "academia-monthly")
	e.event(t, Event{ID: "evt_done", Type: EventCheckoutCompleted, Checkout: &CheckoutEvent{
		CheckoutID: checkout, Mode: "subscription", Paid: true, CustomerID: "cus_1", TaxID: "B12345674",
	}})
	assert.Equal(t, "B12345674", e.saved(t).TaxID)
}

func TestWebhookEvents_IncludeCustomerUpdated(t *testing.T) {
	assert.Contains(t, WebhookEvents, EventCustomerUpdated)
}
