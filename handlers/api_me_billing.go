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
	"github.com/jmaister/taronja-gateway/session"
)

// GetMyBilling handles GET /me/billing (authenticated): get the user's subscriptions and documents.
func (s *StrictApiServer) GetMyBilling(ctx context.Context, request api.GetMyBillingRequestObject) (api.GetMyBillingResponseObject, error) {
	sess, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || !sess.IsAuthenticated {
		return api.GetMyBilling401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.billing == nil {
		return api.GetMyBilling500JSONResponse{Code: http.StatusInternalServerError, Message: "Billing is not configured"}, nil
	}

	resp := api.MyBilling{
		Subscriptions: []api.MyBillingSubscription{},
		Documents:     []api.BillingDocument{},
	}

	// Get user's own subscriptions and documents
	userSubject := billing.UserSubject(sess.UserID)
	subs, err := s.billing.Repo().ListSubscriptions(repositories.SubscriptionFilter{Subject: string(userSubject)})
	if err != nil {
		log.Printf("Error listing user subscriptions: %v", err)
		return api.GetMyBilling500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to load billing information"}, nil
	}
	for _, sub := range subs {
		plan, err := s.billing.GetPlan(sub.PlanKey)
		planName := sub.PlanKey
		if err == nil && plan != nil {
			planName = plan.Name
		}
		resp.Subscriptions = append(resp.Subscriptions, api.MyBillingSubscription{
			Subscription: subscriptionToAPI(&sub),
			PlanName:     planName,
			SubjectName:  "", // user's own name, empty for now
		})
	}

	docs, err := s.billing.Repo().ListDocuments(repositories.DocumentFilter{Subject: string(userSubject)})
	if err != nil {
		log.Printf("Error listing user documents: %v", err)
		return api.GetMyBilling500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to load billing information"}, nil
	}
	for _, doc := range docs {
		resp.Documents = append(resp.Documents, documentToAPI(&doc))
	}

	// Get organizations the user owns
	orgs, err := s.billing.Orgs().OfUser(sess.UserID)
	if err != nil {
		log.Printf("Error listing user organizations: %v", err)
		return api.GetMyBilling500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to load billing information"}, nil
	}
	for _, orgMembership := range orgs {
		if orgMembership.Role != "owner" {
			continue // only include organizations where the user is an owner
		}
		orgSubject := billing.OrgSubject(orgMembership.Organization.ID)
		orgSubs, err := s.billing.Repo().ListSubscriptions(repositories.SubscriptionFilter{Subject: string(orgSubject)})
		if err != nil {
			log.Printf("Error listing org subscriptions for %s: %v", orgMembership.Organization.ID, err)
			continue
		}
		for _, sub := range orgSubs {
			plan, err := s.billing.GetPlan(sub.PlanKey)
			planName := sub.PlanKey
			if err == nil && plan != nil {
				planName = plan.Name
			}
			resp.Subscriptions = append(resp.Subscriptions, api.MyBillingSubscription{
				Subscription: subscriptionToAPI(&sub),
				PlanName:     planName,
				SubjectName:  orgMembership.Organization.Name,
			})
		}

		orgDocs, err := s.billing.Repo().ListDocuments(repositories.DocumentFilter{Subject: string(orgSubject)})
		if err != nil {
			log.Printf("Error listing org documents for %s: %v", orgMembership.Organization.ID, err)
			continue
		}
		for _, doc := range orgDocs {
			resp.Documents = append(resp.Documents, documentToAPI(&doc))
		}
	}

	return api.GetMyBilling200JSONResponse(resp), nil
}

// CreateMyBillingPortal handles POST /me/billing/portal (authenticated): get a Stripe portal URL.
func (s *StrictApiServer) CreateMyBillingPortal(ctx context.Context, request api.CreateMyBillingPortalRequestObject) (api.CreateMyBillingPortalResponseObject, error) {
	sess, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || !sess.IsAuthenticated {
		return api.CreateMyBillingPortal401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.billing == nil {
		return api.CreateMyBillingPortal400JSONResponse{Code: http.StatusBadRequest, Message: "Billing is not configured"}, nil
	}
	if request.Body == nil {
		return api.CreateMyBillingPortal400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}

	subject := request.Body.Subject

	url, err := s.billing.PortalURL(ctx, subject, sess.UserID)
	if errors.Is(err, billing.ErrForbidden) {
		return api.CreateMyBillingPortal403JSONResponse{Code: http.StatusForbidden, Message: "You do not have access to this subject"}, nil
	}
	if errors.Is(err, billing.ErrNotFound) {
		return api.CreateMyBillingPortal404JSONResponse{Code: http.StatusNotFound, Message: "No Stripe customer yet: subscribe first"}, nil
	}
	if errors.Is(err, billing.ErrInvalid) {
		return api.CreateMyBillingPortal400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
	}
	if err != nil {
		log.Printf("Error creating billing portal for %s: %v", subject, err)
		return api.CreateMyBillingPortal500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to create portal URL"}, nil
	}

	return api.CreateMyBillingPortal200JSONResponse(api.BillingPortalResponse{Url: url}), nil
}
