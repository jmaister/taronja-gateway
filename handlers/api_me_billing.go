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
			Subscription: s.subscriptionToAPI(&sub),
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
				Subscription: s.subscriptionToAPI(&sub),
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

	returnPath := ""
	if request.Body.ReturnUrl != nil {
		returnPath = *request.Body.ReturnUrl
	}
	url, err := s.billing.PortalURL(ctx, subject, sess.UserID, returnPath)
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

func customerDetailsToAPI(c *db.BillingCustomer) api.BillingCustomerDetails {
	d := api.BillingCustomerDetails{
		Subject: c.Subject, Name: c.Name, TaxId: c.TaxID, Phone: c.Phone,
		AddressLine1: c.AddressLine1, AddressLine2: c.AddressLine2, PostalCode: c.PostalCode,
		City: c.City, State: c.State, Country: c.Country,
	}
	if c.Kind != "" {
		kind := api.BillingCustomerDetailsKind(c.Kind)
		d.Kind = &kind
	}
	return d
}

// GetMyBillingCustomer handles GET /me/billing/customer (authenticated): the saved invoice details.
func (s *StrictApiServer) GetMyBillingCustomer(ctx context.Context, request api.GetMyBillingCustomerRequestObject) (api.GetMyBillingCustomerResponseObject, error) {
	sess, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || !sess.IsAuthenticated {
		return api.GetMyBillingCustomer401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.billing == nil {
		return api.GetMyBillingCustomer503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	}
	c, err := s.billing.GetCustomerDetails(request.Params.Subject, sess.UserID)
	switch {
	case errors.Is(err, billing.ErrForbidden):
		return api.GetMyBillingCustomer403JSONResponse{Code: http.StatusForbidden, Message: "You do not have access to this subject"}, nil
	case errors.Is(err, billing.ErrInvalid):
		return api.GetMyBillingCustomer400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
	case err != nil:
		log.Printf("Error reading billing details of %s: %v", request.Params.Subject, err)
		return api.GetMyBillingCustomer500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to load billing details"}, nil
	}
	return api.GetMyBillingCustomer200JSONResponse(customerDetailsToAPI(c)), nil
}

// SaveMyBillingCustomer handles POST /me/billing/customer (authenticated): save the invoice details before paying.
func (s *StrictApiServer) SaveMyBillingCustomer(ctx context.Context, request api.SaveMyBillingCustomerRequestObject) (api.SaveMyBillingCustomerResponseObject, error) {
	sess, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || !sess.IsAuthenticated {
		return api.SaveMyBillingCustomer401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.billing == nil {
		return api.SaveMyBillingCustomer503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	}
	if request.Body == nil {
		return api.SaveMyBillingCustomer400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}
	b := request.Body
	payer := billing.Payer{UserID: sess.UserID, Email: sess.Email}
	user, err := s.userRepo.FindUserByIdOrUsername(sess.UserID, "", "")
	if err == nil && user != nil {
		payer.Name, payer.Email = user.Name, user.Email
	}
	d := billing.CustomerDetails{
		Kind: string(b.Kind), Name: b.Name, Phone: deref(b.Phone),
		TaxID: deref(b.TaxId),
		Address: billing.Address{
			Line1: b.AddressLine1, Line2: deref(b.AddressLine2), PostalCode: b.PostalCode,
			City: b.City, State: deref(b.State), Country: b.Country,
		},
	}
	c, err := s.billing.SaveCustomerDetails(ctx, b.Subject, payer, d)
	switch {
	case errors.Is(err, billing.ErrForbidden):
		return api.SaveMyBillingCustomer403JSONResponse{Code: http.StatusForbidden, Message: "You do not have access to this subject"}, nil
	case errors.Is(err, billing.ErrInvalid):
		return api.SaveMyBillingCustomer400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
	case errors.Is(err, billing.ErrConflict):
		return api.SaveMyBillingCustomer409JSONResponse{Code: http.StatusConflict, Message: err.Error()}, nil
	case errors.Is(err, billing.ErrNotConfigured):
		return api.SaveMyBillingCustomer503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Billing is not configured"}, nil
	case err != nil:
		log.Printf("Error saving billing details of %s: %v", b.Subject, err)
		return api.SaveMyBillingCustomer500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to save billing details"}, nil
	}
	return api.SaveMyBillingCustomer200JSONResponse(customerDetailsToAPI(c)), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
