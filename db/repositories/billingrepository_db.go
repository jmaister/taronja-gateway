package repositories

import (
	"errors"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// BillingRepositoryDB implements BillingRepository using GORM.
type BillingRepositoryDB struct {
	db *gorm.DB
}

// NewBillingRepositoryDB creates a new BillingRepositoryDB.
func NewBillingRepositoryDB(gormDB *gorm.DB) *BillingRepositoryDB {
	return &BillingRepositoryDB{db: gormDB}
}

// var _ BillingRepository = (*BillingRepositoryDB)(nil) ensures
// the type satisfies the interface at compile time.
var _ BillingRepository = (*BillingRepositoryDB)(nil)

// Transaction runs fn with a BillingRepositoryDB bound to a database transaction.
// Commits if fn returns nil, rolls back otherwise.
func (r *BillingRepositoryDB) Transaction(fn func(BillingRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(&BillingRepositoryDB{db: tx})
	})
}

// SavePlan creates or replaces a plan and its features in a transaction.
func (r *BillingRepositoryDB) SavePlan(plan *BillingPlanWithFeatures) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete existing plan and features to ensure clean replace
		err := tx.Where("plan_key = ?", plan.Key).Delete(&db.BillingPlanFeature{}).Error
		if err != nil {
			return err
		}
		err = tx.Where("key = ?", plan.Key).Delete(&db.BillingPlan{}).Error
		if err != nil {
			return err
		}

		// GORM leaves zero values of columns with a default out of the
		// INSERT (and reads the defaults back into the struct), so Active
		// false and GraceDays 0 are written by an UPDATE afterwards.
		active, graceDays := plan.Active, plan.GraceDays
		err = tx.Create(&plan.BillingPlan).Error
		if err != nil {
			return err
		}
		plan.Active, plan.GraceDays = active, graceDays
		err = tx.Model(&db.BillingPlan{}).Where("key = ?", plan.Key).
			Updates(map[string]any{"active": active, "grace_days": graceDays}).Error
		if err != nil {
			return err
		}

		// Insert new features
		for _, feature := range plan.Features {
			err := tx.Create(&db.BillingPlanFeature{PlanKey: plan.Key, Feature: feature}).Error
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// GetPlan retrieves a plan with its features.
func (r *BillingRepositoryDB) GetPlan(key string) (*BillingPlanWithFeatures, error) {
	var plan db.BillingPlan
	err := r.db.Where("key = ?", key).First(&plan).Error
	if err != nil {
		return nil, err
	}

	var features []db.BillingPlanFeature
	err = r.db.Where("plan_key = ?", key).Order("feature ASC").Find(&features).Error
	if err != nil {
		return nil, err
	}

	result := &BillingPlanWithFeatures{BillingPlan: plan}
	for _, f := range features {
		result.Features = append(result.Features, f.Feature)
	}

	return result, nil
}

// ListPlans returns all plans (optionally filtered to active only), with features sorted.
func (r *BillingRepositoryDB) ListPlans(activeOnly bool) ([]BillingPlanWithFeatures, error) {
	var plans []db.BillingPlan
	query := r.db
	if activeOnly {
		query = query.Where("active = 1")
	}
	err := query.Order("key ASC").Find(&plans).Error
	if err != nil {
		return nil, err
	}

	var results []BillingPlanWithFeatures
	for _, plan := range plans {
		var features []db.BillingPlanFeature
		err := r.db.Where("plan_key = ?", plan.Key).Order("feature ASC").Find(&features).Error
		if err != nil {
			return nil, err
		}

		planWithFeatures := BillingPlanWithFeatures{BillingPlan: plan}
		for _, f := range features {
			planWithFeatures.Features = append(planWithFeatures.Features, f.Feature)
		}
		results = append(results, planWithFeatures)
	}

	return results, nil
}

// SaveCustomer creates or updates a customer.
func (r *BillingRepositoryDB) SaveCustomer(c *db.BillingCustomer) error {
	return r.db.Save(c).Error
}

// GetCustomer retrieves a customer by subject.
func (r *BillingRepositoryDB) GetCustomer(subject string) (*db.BillingCustomer, error) {
	var c db.BillingCustomer
	err := r.db.Where("subject = ?", subject).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCustomerByStripeID retrieves a customer by Stripe customer ID.
func (r *BillingRepositoryDB) GetCustomerByStripeID(stripeCustomerID string) (*db.BillingCustomer, error) {
	var c db.BillingCustomer
	err := r.db.Where("stripe_customer_id = ?", stripeCustomerID).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateCheckout creates a new checkout.
func (r *BillingRepositoryDB) CreateCheckout(c *db.BillingCheckout) error {
	return r.db.Create(c).Error
}

// GetCheckout retrieves a checkout by id.
func (r *BillingRepositoryDB) GetCheckout(id string) (*db.BillingCheckout, error) {
	var c db.BillingCheckout
	err := r.db.Where("id = ?", id).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetCheckoutByStripeSession retrieves a checkout by Stripe session id.
func (r *BillingRepositoryDB) GetCheckoutByStripeSession(sessionID string) (*db.BillingCheckout, error) {
	var c db.BillingCheckout
	err := r.db.Where("stripe_session_id = ?", sessionID).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCheckout updates an existing checkout.
func (r *BillingRepositoryDB) UpdateCheckout(c *db.BillingCheckout) error {
	return r.db.Save(c).Error
}

// CreateSubscription creates a new subscription, returning ErrDuplicate
// if a subscription with the same StripeSubscriptionID already exists.
func (r *BillingRepositoryDB) CreateSubscription(s *db.BillingSubscription) error {
	err := r.db.Create(s).Error
	if err != nil {
		if isDuplicateError(err) {
			return ErrDuplicate
		}
	}
	return err
}

// GetSubscription retrieves a subscription by id.
func (r *BillingRepositoryDB) GetSubscription(id string) (*db.BillingSubscription, error) {
	var s db.BillingSubscription
	err := r.db.Where("id = ?", id).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSubscriptionByStripeID retrieves a subscription by Stripe subscription ID.
func (r *BillingRepositoryDB) GetSubscriptionByStripeID(stripeSubscriptionID string) (*db.BillingSubscription, error) {
	var s db.BillingSubscription
	err := r.db.Where("stripe_subscription_id = ?", stripeSubscriptionID).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateSubscription updates an existing subscription.
func (r *BillingRepositoryDB) UpdateSubscription(s *db.BillingSubscription) error {
	return r.db.Save(s).Error
}

// ListSubscriptions returns subscriptions matching the filter, newest first.
func (r *BillingRepositoryDB) ListSubscriptions(filter SubscriptionFilter) ([]db.BillingSubscription, error) {
	query := r.db
	if filter.Subject != "" {
		query = query.Where("subject = ?", filter.Subject)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.PlanKey != "" {
		query = query.Where("plan_key = ?", filter.PlanKey)
	}

	var subscriptions []db.BillingSubscription
	err := query.Order("created_at DESC, id DESC").Find(&subscriptions).Error
	return subscriptions, err
}

// ListSubscriptionsBySubjects returns subscriptions for any of the given subjects.
func (r *BillingRepositoryDB) ListSubscriptionsBySubjects(subjects []string) ([]db.BillingSubscription, error) {
	var subscriptions []db.BillingSubscription
	err := r.db.Where("subject IN ?", subjects).Order("created_at DESC, id DESC").Find(&subscriptions).Error
	return subscriptions, err
}

// CreatePurchase creates a new purchase, returning ErrDuplicate if
// a purchase with the same StripePaymentIntentID already exists.
func (r *BillingRepositoryDB) CreatePurchase(p *db.BillingPurchase) error {
	err := r.db.Create(p).Error
	if err != nil {
		if isDuplicateError(err) {
			return ErrDuplicate
		}
	}
	return err
}

// GetPurchase retrieves a purchase by id.
func (r *BillingRepositoryDB) GetPurchase(id string) (*db.BillingPurchase, error) {
	var p db.BillingPurchase
	err := r.db.Where("id = ?", id).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPurchaseByPaymentIntent retrieves a purchase by Stripe payment intent ID.
func (r *BillingRepositoryDB) GetPurchaseByPaymentIntent(paymentIntentID string) (*db.BillingPurchase, error) {
	var p db.BillingPurchase
	err := r.db.Where("stripe_payment_intent_id = ?", paymentIntentID).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdatePurchase updates an existing purchase.
func (r *BillingRepositoryDB) UpdatePurchase(p *db.BillingPurchase) error {
	return r.db.Save(p).Error
}

// ListPurchases returns purchases matching the filter, newest paid first.
func (r *BillingRepositoryDB) ListPurchases(filter PurchaseFilter) ([]db.BillingPurchase, error) {
	query := r.db
	if filter.Subject != "" {
		query = query.Where("subject = ?", filter.Subject)
	}
	if filter.PlanKey != "" {
		query = query.Where("plan_key = ?", filter.PlanKey)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Since != nil {
		query = query.Where("paid_at >= ?", filter.Since.UTC())
	}

	var purchases []db.BillingPurchase
	err := query.Order("paid_at DESC, id DESC").Find(&purchases).Error
	return purchases, err
}

// ListPurchasesBySubjects returns purchases for any of the given subjects.
func (r *BillingRepositoryDB) ListPurchasesBySubjects(subjects []string) ([]db.BillingPurchase, error) {
	var purchases []db.BillingPurchase
	err := r.db.Where("subject IN ?", subjects).Order("paid_at DESC, id DESC").Find(&purchases).Error
	return purchases, err
}

// CreateGrant creates a new grant.
func (r *BillingRepositoryDB) CreateGrant(g *db.BillingGrant) error {
	return r.db.Create(g).Error
}

// DeleteGrant deletes a grant, returning gorm.ErrRecordNotFound if not found.
func (r *BillingRepositoryDB) DeleteGrant(id string) error {
	result := r.db.Where("id = ?", id).Delete(&db.BillingGrant{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListGrants returns all grants (or for a specific subject), newest first.
func (r *BillingRepositoryDB) ListGrants(subject string) ([]db.BillingGrant, error) {
	query := r.db
	if subject != "" {
		query = query.Where("subject = ?", subject)
	}

	var grants []db.BillingGrant
	err := query.Order("created_at DESC, id DESC").Find(&grants).Error
	return grants, err
}

// ListGrantsBySubjects returns grants for any of the given subjects.
func (r *BillingRepositoryDB) ListGrantsBySubjects(subjects []string) ([]db.BillingGrant, error) {
	var grants []db.BillingGrant
	err := r.db.Where("subject IN ?", subjects).Order("created_at DESC, id DESC").Find(&grants).Error
	return grants, err
}

// CreateDocument creates a new document, returning ErrDuplicate if
// a document with the same StripeRef already exists.
func (r *BillingRepositoryDB) CreateDocument(d *db.BillingDocument) error {
	err := r.db.Create(d).Error
	if err != nil {
		if isDuplicateError(err) {
			return ErrDuplicate
		}
	}
	return err
}

// GetDocument retrieves a document by id.
func (r *BillingRepositoryDB) GetDocument(id string) (*db.BillingDocument, error) {
	var d db.BillingDocument
	err := r.db.Where("id = ?", id).First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// GetDocumentByStripeRef retrieves a document by Stripe reference.
func (r *BillingRepositoryDB) GetDocumentByStripeRef(stripeRef string) (*db.BillingDocument, error) {
	var d db.BillingDocument
	err := r.db.Where("stripe_ref = ?", stripeRef).First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UpdateDocument updates an existing document.
func (r *BillingRepositoryDB) UpdateDocument(d *db.BillingDocument) error {
	return r.db.Save(d).Error
}

// DueDocuments returns pending documents whose NextAttemptAt is not after now,
// oldest first, at most limit.
func (r *BillingRepositoryDB) DueDocuments(now time.Time, limit int) ([]db.BillingDocument, error) {
	var documents []db.BillingDocument
	err := r.db.
		Where("odoo_state = ? AND next_attempt_at <= ?", db.OdooStatePending, now.UTC()).
		Order("next_attempt_at ASC, created_at ASC").
		Limit(limit).
		Find(&documents).Error
	return documents, err
}

// ListDocuments returns documents matching the filter, newest first.
func (r *BillingRepositoryDB) ListDocuments(filter DocumentFilter) ([]db.BillingDocument, error) {
	query := r.db
	if filter.Subject != "" {
		query = query.Where("subject = ?", filter.Subject)
	}
	if filter.OdooState != "" {
		query = query.Where("odoo_state = ?", filter.OdooState)
	}
	if filter.Kind != "" {
		query = query.Where("kind = ?", filter.Kind)
	}

	var documents []db.BillingDocument
	err := query.Order("created_at DESC, id DESC").Find(&documents).Error
	return documents, err
}

// CreditedCents returns the sum of credit notes for an invoice document.
func (r *BillingRepositoryDB) CreditedCents(invoiceDocumentID string) (int64, error) {
	var sum int64
	err := r.db.
		Model(&db.BillingDocument{}).
		Where("kind = ? AND invoice_document_id = ?", db.DocumentCreditNote, invoiceDocumentID).
		Select("COALESCE(SUM(amount_cents), 0)").
		Row().
		Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// RecordStripeEvent stores a Stripe event and returns true if it was new,
// false if it was already processed (idempotent).
func (r *BillingRepositoryDB) RecordStripeEvent(eventID, eventType string) (bool, error) {
	result := r.db.Create(&db.StripeEvent{
		EventID: eventID,
		Type:    eventType,
	})

	// If there's no error, it's a new event
	if result.Error == nil {
		return true, nil
	}

	// If it's a duplicate constraint error, the event was already processed
	if isDuplicateError(result.Error) {
		return false, nil
	}

	// Any other error is returned
	return false, result.Error
}

// isDuplicateError checks if an error is a duplicate key constraint error.
// It checks for both GORM's ErrDuplicatedKey and SQLite's "UNIQUE constraint failed" message.
func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}

	// Check for gorm.ErrDuplicatedKey
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	// Check for SQLite's "UNIQUE constraint failed" message
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return true
	}

	return false
}
