package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
)

// ListBillingPurchases handles GET /api/billing/purchases (admin): list purchases.
func (s *StrictApiServer) ListBillingPurchases(ctx context.Context, request api.ListBillingPurchasesRequestObject) (api.ListBillingPurchasesResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.ListBillingPurchases401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.ListBillingPurchases403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.ListBillingPurchases500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	filter := repositories.PurchaseFilter{}
	if request.Params.Subject != nil {
		filter.Subject = *request.Params.Subject
	}
	if request.Params.Plan != nil {
		filter.PlanKey = *request.Params.Plan
	}
	if request.Params.Status != nil {
		filter.Status = *request.Params.Status
	}
	if request.Params.Since != nil {
		filter.Since = request.Params.Since
	}

	purchases, err := s.billing.Repo().ListPurchases(filter)
	if err != nil {
		log.Printf("Error listing billing purchases: %v", err)
		return api.ListBillingPurchases500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list purchases"}, nil
	}

	resp := make(api.ListBillingPurchases200JSONResponse, 0, len(purchases))
	for _, p := range purchases {
		resp = append(resp, purchaseToAPI(&p))
	}
	return resp, nil
}

// RefundBillingPurchase handles POST /api/billing/purchases/{id}/refund (admin): refund a purchase.
func (s *StrictApiServer) RefundBillingPurchase(ctx context.Context, request api.RefundBillingPurchaseRequestObject) (api.RefundBillingPurchaseResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.RefundBillingPurchase401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.RefundBillingPurchase403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.RefundBillingPurchase503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	}

	amountCents := int64(0)
	cancelSubscription := false
	if request.Body != nil {
		if request.Body.AmountCents != nil {
			amountCents = *request.Body.AmountCents
		}
		if request.Body.CancelSubscription != nil {
			cancelSubscription = *request.Body.CancelSubscription
		}
	}

	purchase, err := s.billing.RefundPurchase(ctx, request.Id, amountCents, cancelSubscription)
	if errors.Is(err, billing.ErrNotFound) {
		return api.RefundBillingPurchase404JSONResponse{Code: http.StatusNotFound, Message: "Purchase not found"}, nil
	}
	if errors.Is(err, billing.ErrInvalid) {
		return api.RefundBillingPurchase400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
	}
	if errors.Is(err, billing.ErrConflict) {
		return api.RefundBillingPurchase409JSONResponse{Code: http.StatusConflict, Message: err.Error()}, nil
	}
	if errors.Is(err, billing.ErrNotConfigured) {
		return api.RefundBillingPurchase503JSONResponse{Code: http.StatusServiceUnavailable, Message: err.Error()}, nil
	}
	if err != nil {
		log.Printf("Error refunding billing purchase %s: %v", request.Id, err)
		return api.RefundBillingPurchase500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to refund purchase"}, nil
	}

	return api.RefundBillingPurchase200JSONResponse(purchaseToAPI(purchase)), nil
}

// purchaseToAPI converts a db.BillingPurchase to an API response.
func purchaseToAPI(p *db.BillingPurchase) api.BillingPurchase {
	var shipping *api.ShippingAddress
	if p.ShippingName != "" || p.ShippingPhone != "" || p.ShippingLine1 != "" {
		shipping = &api.ShippingAddress{
			Name:       stringPtr(p.ShippingName),
			Phone:      stringPtr(p.ShippingPhone),
			Line1:      stringPtr(p.ShippingLine1),
			Line2:      stringPtr(p.ShippingLine2),
			City:       stringPtr(p.ShippingCity),
			State:      stringPtr(p.ShippingState),
			PostalCode: stringPtr(p.ShippingPostalCode),
			Country:    stringPtr(p.ShippingCountry),
		}
	}
	return api.BillingPurchase{
		Id:             p.ID,
		Subject:        p.Subject,
		PlanKey:        p.PlanKey,
		PayerUserId:    p.PayerUserID,
		Status:         api.BillingPurchaseStatus(p.Status),
		AmountCents:    p.AmountCents,
		Currency:       p.Currency,
		RefundedCents:  p.RefundedCents,
		SubscriptionId: p.SubscriptionID,
		Shipping:       shipping,
		PaidAt:         p.PaidAt,
	}
}
