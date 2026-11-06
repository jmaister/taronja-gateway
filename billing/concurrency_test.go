package billing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func inParallel(n int, f func(i int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			f(i)
		}()
	}
	close(start)
	wg.Wait()
}

func TestRefundPurchase_DoubleClickRefundsOnce(t *testing.T) {
	e := newEnv(t)
	p := &db.BillingPurchase{Subject: "user:u1", PlanKey: "x", Status: db.PurchaseStatusPaid, AmountCents: 1000, StripePaymentIntentID: "pi_race", PaidAt: e.clock}
	require.NoError(t, e.repo.CreatePurchase(p))

	inParallel(2, func(int) {
		_, err := e.svc.RefundPurchase(context.Background(), p.ID, 500, false)
		assert.NoError(t, err)
	})

	e.stripe.mu.Lock()
	defer e.stripe.mu.Unlock()
	assert.Len(t, e.stripe.Refunds, 1, "both clicks carry the same idempotency key")
}

func TestActivateAndCancelTogetherLeaveNothingAliveInStripe(t *testing.T) {
	for range 5 {
		e := newEnv(t)
		sub, _ := e.awaitingActivation(t)

		inParallel(2, func(i int) {
			if i == 0 {
				_, _ = e.svc.ActivateSubscription(context.Background(), sub.ID)
			} else {
				_, _ = e.svc.CancelSubscription(context.Background(), sub.ID, false)
			}
		})

		got, err := e.repo.GetSubscription(sub.ID)
		require.NoError(t, err)
		e.stripe.mu.Lock()
		for id, s := range e.stripe.Subscriptions {
			if got.Status == db.SubscriptionCanceled {
				assert.Equal(t, "canceled", s.Status, "Stripe subscription %s would keep charging a canceled row", id)
			}
		}
		e.stripe.mu.Unlock()
	}
}

func TestProcessDueDocumentsInParallelIssuesOnce(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "B12345674")
	d := e.invoice(t, "org:o1", "in_par")

	inParallel(3, func(int) {
		_, err := e.svc.ProcessDueDocuments(context.Background())
		assert.NoError(t, err)
	})

	got := e.doc(t, d.ID)
	assert.Equal(t, db.OdooStateDone, got.OdooState)
	e.odoo.mu.Lock()
	defer e.odoo.mu.Unlock()
	assert.Len(t, e.odoo.Moves, 1, "Odoo is idempotent by ref: one invoice however many workers")
}

func TestRunOutbox(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreatePlan(context.Background(), monthlyInput())
	require.NoError(t, err)
	e.customer(t, "org:o1", "B12345674")
	d := e.invoice(t, "org:o1", "in_run")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.svc.RunOutbox(ctx, 5*time.Millisecond)
		close(done)
	}()

	require.Eventually(t, func() bool { return e.doc(t, d.ID).OdooState == db.OdooStateDone }, 5*time.Second, 10*time.Millisecond)
	late := e.invoice(t, "org:o1", "in_run_2")
	require.Eventually(t, func() bool { return e.doc(t, late.ID).OdooState == db.OdooStateDone }, 5*time.Second, 10*time.Millisecond, "a document queued later is picked up by the next tick")

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunOutbox did not stop with its context")
	}
}

func TestRunNotifierStopsWithItsContext(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.svc.RunNotifier(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunNotifier did not stop with its context")
	}
}
