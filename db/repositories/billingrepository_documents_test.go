package repositories

import (
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingRepositoryDB_CreateDocument(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	doc := &db.BillingDocument{
		Subject:       "user123",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_123",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now.Add(time.Hour),
	}

	err := repo.CreateDocument(doc)
	require.NoError(t, err)
	assert.NotEmpty(t, doc.ID)
}

func TestBillingRepositoryDB_CreateDocument_DuplicateStripeRef(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	doc1 := &db.BillingDocument{
		Subject:       "user123",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_123",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now.Add(time.Hour),
	}
	require.NoError(t, repo.CreateDocument(doc1))

	// Try to create with same StripeRef
	doc2 := &db.BillingDocument{
		Subject:       "user456",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_123",
		AmountCents:   5000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now.Add(time.Hour),
	}
	err := repo.CreateDocument(doc2)
	assert.ErrorIs(t, err, ErrDuplicate)
}

func TestBillingRepositoryDB_GetDocument(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	doc := &db.BillingDocument{
		Subject:       "user123",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_456",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now.Add(time.Hour),
	}
	require.NoError(t, repo.CreateDocument(doc))

	retrieved, err := repo.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, doc.ID, retrieved.ID)
	assert.Equal(t, db.DocumentInvoice, retrieved.Kind)
}

func TestBillingRepositoryDB_GetDocumentByStripeRef(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	doc := &db.BillingDocument{
		Subject:       "user123",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_789",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now.Add(time.Hour),
	}
	require.NoError(t, repo.CreateDocument(doc))

	retrieved, err := repo.GetDocumentByStripeRef("inv_789")
	require.NoError(t, err)
	assert.Equal(t, doc.ID, retrieved.ID)
}

func TestBillingRepositoryDB_UpdateDocument(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	doc := &db.BillingDocument{
		Subject:       "user123",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_123",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now.Add(time.Hour),
		Attempts:      0,
	}
	require.NoError(t, repo.CreateDocument(doc))

	doc.OdooState = db.OdooStateDone
	doc.Attempts = 1
	err := repo.UpdateDocument(doc)
	require.NoError(t, err)

	retrieved, err := repo.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, db.OdooStateDone, retrieved.OdooState)
	assert.Equal(t, 1, retrieved.Attempts)
}

func TestBillingRepositoryDB_DueDocuments(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	// Create a document that's due (in the past)
	doc1 := &db.BillingDocument{
		Subject:       "user1",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_1",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        past,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: past,
	}
	require.NoError(t, repo.CreateDocument(doc1))

	// Create a document that's not due yet (in the future)
	doc2 := &db.BillingDocument{
		Subject:       "user2",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_2",
		AmountCents:   5000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: future,
	}
	require.NoError(t, repo.CreateDocument(doc2))

	// Create a document that's not pending
	doc3 := &db.BillingDocument{
		Subject:       "user3",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_3",
		AmountCents:   7000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        past,
		OdooState:     db.OdooStateDone,
		NextAttemptAt: past,
	}
	require.NoError(t, repo.CreateDocument(doc3))

	due, err := repo.DueDocuments(now, 10)
	require.NoError(t, err)
	assert.Len(t, due, 1)
	assert.Equal(t, doc1.ID, due[0].ID)
}

func TestBillingRepositoryDB_ListDocuments(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	doc1 := &db.BillingDocument{
		Subject:       "user1",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_1",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now,
	}
	doc2 := &db.BillingDocument{
		Subject:       "user2",
		Kind:          db.DocumentCreditNote,
		StripeRef:     "cn_1",
		AmountCents:   5000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStateDone,
		NextAttemptAt: now,
	}
	require.NoError(t, repo.CreateDocument(doc1))
	require.NoError(t, repo.CreateDocument(doc2))

	docs, err := repo.ListDocuments(DocumentFilter{})
	require.NoError(t, err)
	assert.Len(t, docs, 2)
	// Newest first
	assert.Equal(t, doc2.ID, docs[0].ID)
	assert.Equal(t, doc1.ID, docs[1].ID)
}

func TestBillingRepositoryDB_ListDocuments_FilterBySubject(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	doc1 := &db.BillingDocument{
		Subject:       "user1",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_1",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now,
	}
	doc2 := &db.BillingDocument{
		Subject:       "user2",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_2",
		AmountCents:   5000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStatePending,
		NextAttemptAt: now,
	}
	require.NoError(t, repo.CreateDocument(doc1))
	require.NoError(t, repo.CreateDocument(doc2))

	docs, err := repo.ListDocuments(DocumentFilter{Subject: "user1"})
	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "user1", docs[0].Subject)
}

func TestBillingRepositoryDB_CreditedCents(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	now := time.Now().UTC()
	invoiceDocID := "doc_123"

	// Create an invoice
	invoice := &db.BillingDocument{
		Subject:       "user1",
		Kind:          db.DocumentInvoice,
		StripeRef:     "inv_1",
		AmountCents:   10000,
		Currency:      "eur",
		Lines:         "[]",
		PaidAt:        now,
		OdooState:     db.OdooStateDone,
		NextAttemptAt: now,
	}
	require.NoError(t, repo.CreateDocument(invoice))

	// Create credit notes for this invoice
	cn1 := &db.BillingDocument{
		Subject:           "user1",
		Kind:              db.DocumentCreditNote,
		StripeRef:         "cn_1",
		AmountCents:       3000,
		Currency:          "eur",
		Lines:             "[]",
		PaidAt:            now,
		OdooState:         db.OdooStateDone,
		NextAttemptAt:     now,
		InvoiceDocumentID: &invoiceDocID,
	}
	cn2 := &db.BillingDocument{
		Subject:           "user1",
		Kind:              db.DocumentCreditNote,
		StripeRef:         "cn_2",
		AmountCents:       2000,
		Currency:          "eur",
		Lines:             "[]",
		PaidAt:            now,
		OdooState:         db.OdooStateDone,
		NextAttemptAt:     now,
		InvoiceDocumentID: &invoiceDocID,
	}
	require.NoError(t, repo.CreateDocument(cn1))
	require.NoError(t, repo.CreateDocument(cn2))

	credited, err := repo.CreditedCents(invoiceDocID)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), credited)
}

func TestBillingRepositoryDB_CreditedCents_NoCreditNotes(t *testing.T) {
	db.SetupTestDB(t.Name())
	conn := db.GetConnection()
	repo := NewBillingRepositoryDB(conn)

	credited, err := repo.CreditedCents("nonexistent_invoice")
	require.NoError(t, err)
	assert.Equal(t, int64(0), credited)
}
