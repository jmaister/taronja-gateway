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

// ListBillingSubscriptions handles GET /api/billing/subscriptions (admin): list subscriptions.
func (s *StrictApiServer) ListBillingSubscriptions(ctx context.Context, request api.ListBillingSubscriptionsRequestObject) (api.ListBillingSubscriptionsResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.ListBillingSubscriptions401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.ListBillingSubscriptions403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.ListBillingSubscriptions500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	filter := repositories.SubscriptionFilter{}
	if request.Params.Subject != nil {
		filter.Subject = *request.Params.Subject
	}
	if request.Params.Status != nil {
		filter.Status = *request.Params.Status
	}
	if request.Params.Plan != nil {
		filter.PlanKey = *request.Params.Plan
	}

	subs, err := s.billing.Repo().ListSubscriptions(filter)
	if err != nil {
		log.Printf("Error listing billing subscriptions: %v", err)
		return api.ListBillingSubscriptions500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list subscriptions"}, nil
	}

	resp := make(api.ListBillingSubscriptions200JSONResponse, 0, len(subs))
	for _, sub := range subs {
		resp = append(resp, subscriptionToAPI(&sub))
	}
	return resp, nil
}

// ActivateBillingSubscription handles POST /api/billing/subscriptions/{id}/activate (admin): activate a subscription.
func (s *StrictApiServer) ActivateBillingSubscription(ctx context.Context, request api.ActivateBillingSubscriptionRequestObject) (api.ActivateBillingSubscriptionResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.ActivateBillingSubscription401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.ActivateBillingSubscription403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.ActivateBillingSubscription503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	}

	sub, err := s.billing.ActivateSubscription(ctx, request.Id)
	if errors.Is(err, billing.ErrNotFound) {
		return api.ActivateBillingSubscription404JSONResponse{Code: http.StatusNotFound, Message: "Subscription not found"}, nil
	}
	if errors.Is(err, billing.ErrConflict) {
		return api.ActivateBillingSubscription409JSONResponse{Code: http.StatusConflict, Message: err.Error()}, nil
	}
	if errors.Is(err, billing.ErrNotConfigured) {
		return api.ActivateBillingSubscription503JSONResponse{Code: http.StatusServiceUnavailable, Message: err.Error()}, nil
	}
	if err != nil {
		log.Printf("Error activating billing subscription %s: %v", request.Id, err)
		return api.ActivateBillingSubscription500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to activate subscription"}, nil
	}

	return api.ActivateBillingSubscription200JSONResponse(subscriptionToAPI(sub)), nil
}

// CancelBillingSubscription handles POST /api/billing/subscriptions/{id}/cancel (admin): cancel a subscription.
func (s *StrictApiServer) CancelBillingSubscription(ctx context.Context, request api.CancelBillingSubscriptionRequestObject) (api.CancelBillingSubscriptionResponseObject, error) {
	switch _, access := requireAdminAccess(ctx); access {
	case accessUnauthenticated:
		return api.CancelBillingSubscription401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	case accessForbidden:
		return api.CancelBillingSubscription403JSONResponse{Code: http.StatusForbidden, Message: "Admin access required"}, nil
	}
	if s.billing == nil {
		return api.CancelBillingSubscription503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	}

	atPeriodEnd := false
	if request.Body != nil {
		atPeriodEnd = request.Body.AtPeriodEnd
	}

	sub, err := s.billing.CancelSubscription(ctx, request.Id, atPeriodEnd)
	if errors.Is(err, billing.ErrNotFound) {
		return api.CancelBillingSubscription404JSONResponse{Code: http.StatusNotFound, Message: "Subscription not found"}, nil
	}
	if errors.Is(err, billing.ErrNotConfigured) {
		return api.CancelBillingSubscription503JSONResponse{Code: http.StatusServiceUnavailable, Message: err.Error()}, nil
	}
	if err != nil {
		log.Printf("Error canceling billing subscription %s: %v", request.Id, err)
		return api.CancelBillingSubscription500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to cancel subscription"}, nil
	}

	return api.CancelBillingSubscription200JSONResponse(subscriptionToAPI(sub)), nil
}

// subscriptionToAPI converts a db.BillingSubscription to an API response.
func subscriptionToAPI(sub *db.BillingSubscription) api.BillingSubscription {
	return api.BillingSubscription{
		Id:                   sub.ID,
		Subject:              sub.Subject,
		PlanKey:              sub.PlanKey,
		PayerUserId:          sub.PayerUserID,
		Status:               api.BillingSubscriptionStatus(sub.Status),
		StripeSubscriptionId: sub.StripeSubscriptionID,
		TrialEnd:             sub.TrialEnd,
		CurrentPeriodEnd:     sub.CurrentPeriodEnd,
		CancelAtPeriodEnd:    sub.CancelAtPeriodEnd,
		GraceUntil:           sub.GraceUntil,
		ActivatedAt:          sub.ActivatedAt,
		CanceledAt:           sub.CanceledAt,
		CreatedAt:            sub.CreatedAt,
	}
}
