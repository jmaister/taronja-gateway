package dbmigrations_test

import (
	"io/fs"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/require"
)

// A database that finished the legacy bridge before billing existed gets the
// tables only from 0006, so its SQL must give tables GORM can use.
func TestBillingMigration_CreatesUsableTables(t *testing.T) {
	db.SetupTestDB(t.Name())
	gdb := db.GetConnection()
	for _, table := range []string{"organizations", "organization_members", "billing_plans", "billing_plan_features", "billing_customers", "billing_checkouts", "billing_subscriptions", "billing_purchases", "billing_grants", "billing_documents", "stripe_events"} {
		require.NoError(t, gdb.Exec("DROP TABLE "+table).Error, table)
	}
	sqlText, err := fs.ReadFile(db.MigrationSchema().Migrations, "0006_billing.up.sql")
	require.NoError(t, err)
	require.NoError(t, gdb.Exec(string(sqlText)).Error)

	org := &db.Organization{Name: "School"}
	require.NoError(t, gdb.Create(org).Error)
	require.NoError(t, gdb.Create(&db.OrganizationMember{OrganizationID: org.ID, UserID: "u1", Role: db.OrganizationRoleOwner}).Error)
	require.NoError(t, gdb.Create(&db.BillingPlan{Key: "p", Name: "P", Kind: db.PlanKindSubscription, Interval: db.PlanIntervalMonth, AmountCents: 1000}).Error)
	var plan db.BillingPlan
	require.NoError(t, gdb.First(&plan, "`key` = ?", "p").Error)
	require.Equal(t, 7, plan.GraceDays, "SQL defaults match the model")
	require.True(t, plan.Active)
	require.NoError(t, gdb.Create(&db.BillingPlanFeature{PlanKey: "p", Feature: "app.x"}).Error)
	require.NoError(t, gdb.Create(&db.BillingCustomer{Subject: "org:" + org.ID}).Error)
	require.NoError(t, gdb.Create(&db.BillingCheckout{Subject: "org:" + org.ID, PlanKey: "p", UserID: "u1", Status: db.CheckoutStatusOpen}).Error)
	require.NoError(t, gdb.Create(&db.BillingSubscription{Subject: "org:" + org.ID, PlanKey: "p", Status: db.SubscriptionActive}).Error)
	require.NoError(t, gdb.Create(&db.BillingPurchase{Subject: "user:u1", PlanKey: "p", Status: db.PurchaseStatusPaid, AmountCents: 1, StripePaymentIntentID: "pi_1", PaidAt: time.Now()}).Error)
	require.NoError(t, gdb.Create(&db.BillingGrant{Subject: "org:" + org.ID, Feature: "app.x"}).Error)
	require.NoError(t, gdb.Create(&db.BillingDocument{Subject: "org:" + org.ID, Kind: db.DocumentInvoice, StripeRef: "in_1", AmountCents: 1, Lines: "[]", PaidAt: time.Now(), OdooState: db.OdooStatePending, NextAttemptAt: time.Now()}).Error)
	require.NoError(t, gdb.Create(&db.StripeEvent{EventID: "evt_1", Type: "invoice.paid"}).Error)
	require.Error(t, gdb.Create(&db.StripeEvent{EventID: "evt_1", Type: "invoice.paid"}).Error, "event ids are unique")
}
