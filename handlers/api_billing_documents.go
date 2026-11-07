package handlers

import (
	"context"
	"errors"
	"log"
	"mime"
	"net/http"
	"strconv"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"gorm.io/gorm"
)

// ListBillingDocuments handles GET /api/billing/documents (admin): list documents.
func (s *StrictApiServer) ListBillingDocuments(ctx context.Context, request api.ListBillingDocumentsRequestObject) (api.ListBillingDocumentsResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.ListBillingDocuments401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.ListBillingDocuments403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.ListBillingDocuments500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	filter := repositories.DocumentFilter{}
	if request.Params.Subject != nil {
		filter.Subject = *request.Params.Subject
	}
	if request.Params.State != nil {
		filter.OdooState = *request.Params.State
	}
	if request.Params.Kind != nil {
		filter.Kind = *request.Params.Kind
	}

	documents, err := s.billing.Repo().ListDocuments(filter)
	if err != nil {
		log.Printf("Error listing billing documents: %v", err)
		return api.ListBillingDocuments500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list documents"}, nil
	}

	resp := make(api.ListBillingDocuments200JSONResponse, 0, len(documents))
	for i := range documents {
		resp = append(resp, s.adminDocumentToAPI(&documents[i]))
	}
	return resp, nil
}

// RetryBillingDocument handles POST /api/billing/documents/{id}/retry (admin): retry a failed document.
func (s *StrictApiServer) RetryBillingDocument(ctx context.Context, request api.RetryBillingDocumentRequestObject) (api.RetryBillingDocumentResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.RetryBillingDocument401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.RetryBillingDocument403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.RetryBillingDocument500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	doc, err := s.billing.RetryDocument(request.Id)
	if errors.Is(err, billing.ErrNotFound) {
		return api.RetryBillingDocument404JSONResponse{Code: http.StatusNotFound, Message: "Document not found"}, nil
	}
	if err != nil {
		log.Printf("Error retrying document %s: %v", request.Id, err)
		return api.RetryBillingDocument500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to retry document"}, nil
	}

	return api.RetryBillingDocument200JSONResponse(s.adminDocumentToAPI(doc)), nil
}

// pdfResponse is the PDF with its filename for the browser.
type pdfResponse struct {
	filename string
	data     []byte
}

func (r pdfResponse) VisitGetBillingDocumentPdfResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": r.filename}))
	w.Header().Set("Content-Length", strconv.Itoa(len(r.data)))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(r.data)
	return err
}

// GetBillingDocumentPdf handles GET /api/billing/documents/{id}/pdf: the PDF
// Odoo stored, for admins, the user invoiced, the payer and the owners of
// the organization invoiced.
func (s *StrictApiServer) GetBillingDocumentPdf(ctx context.Context, request api.GetBillingDocumentPdfRequestObject) (api.GetBillingDocumentPdfResponseObject, error) {
	sessionObj, access := requireAdminAccess(ctx)
	if access == accessUnauthenticated {
		return api.GetBillingDocumentPdf401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.billing == nil {
		return api.GetBillingDocumentPdf503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	}
	doc, err := s.billing.Repo().GetDocument(request.Id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.GetBillingDocumentPdf404JSONResponse{Code: http.StatusNotFound, Message: "Document not found"}, nil
	}
	if err != nil {
		log.Printf("Error reading document %s: %v", request.Id, err)
		return api.GetBillingDocumentPdf500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to get document"}, nil
	}
	if access == accessForbidden {
		allowed, err := s.billing.CanViewDocument(sessionObj.UserID, doc)
		if err != nil {
			log.Printf("Error checking access to document %s: %v", doc.ID, err)
			return api.GetBillingDocumentPdf500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to get document"}, nil
		}
		if !allowed {
			return api.GetBillingDocumentPdf403JSONResponse{Code: http.StatusForbidden, Message: "Not your document"}, nil
		}
	}
	filename, data, err := s.billing.DocumentPDF(ctx, doc.ID)
	if errors.Is(err, billing.ErrPDFNotFound) {
		return api.GetBillingDocumentPdf404JSONResponse{Code: http.StatusNotFound, Message: "The document has no PDF yet"}, nil
	}
	if err != nil {
		log.Printf("Error reading the PDF of document %s: %v", doc.ID, err)
		return api.GetBillingDocumentPdf503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Odoo is not available"}, nil
	}
	return pdfResponse{filename: filename, data: data}, nil
}

// adminDocumentToAPI adds what only admins see: why a credit note is stuck.
func (s *StrictApiServer) adminDocumentToAPI(doc *db.BillingDocument) api.BillingDocument {
	resp := documentToAPI(doc)
	if reason := s.billing.BlockedReason(doc); reason != "" {
		resp.BlockedReason = &reason
	}
	return resp
}

// Helper to convert BillingDocument to API response
func documentToAPI(doc *db.BillingDocument) api.BillingDocument {
	resp := api.BillingDocument{
		Id:          doc.ID,
		Subject:     doc.Subject,
		Kind:        api.BillingDocumentKind(doc.Kind),
		StripeRef:   doc.StripeRef,
		AmountCents: doc.AmountCents,
		Currency:    doc.Currency,
		OdooState:   api.BillingDocumentOdooState(doc.OdooState),
		OdooError:   doc.OdooError,
		PaidAt:      doc.PaidAt,
		IssuedAt:    doc.IssuedAt,
		Number:      doc.Number,
		Attempts:    doc.Attempts,
	}

	if doc.InvoiceDocumentID != nil && *doc.InvoiceDocumentID != "" {
		resp.InvoiceDocumentId = doc.InvoiceDocumentID
	}

	// Convert lines: Lines is a JSON string that needs to be decoded
	lines, err := billing.DocumentLines(doc)
	if err == nil && len(lines) > 0 {
		apiLines := make([]struct {
			AmountCents int64  `json:"amountCents"`
			Description string `json:"description"`
		}, len(lines))
		for i, line := range lines {
			apiLines[i].AmountCents = line.AmountCents
			apiLines[i].Description = line.Description
		}
		resp.Lines = apiLines
	}

	return resp
}
