package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/notification"
	"github.com/jmaister/taronja-gateway/session"
)

// toAPINotification converts a stored db.Notification (with its
// caller-opaque Metadata/Actions stored as raw JSON strings) into the
// OpenAPI-generated shape, decoding both back to structured values.
func toAPINotification(n *db.Notification) (api.Notification, error) {
	metadata, err := notification.DecodeMetadata(n.Metadata)
	if err != nil {
		return api.Notification{}, fmt.Errorf("decoding metadata for notification %s: %w", n.ID, err)
	}
	actions, err := notification.DecodeActions(n.Actions)
	if err != nil {
		return api.Notification{}, fmt.Errorf("decoding actions for notification %s: %w", n.ID, err)
	}

	result := api.Notification{
		Id:                n.ID,
		UserId:            n.UserID,
		Type:              n.Type,
		Title:             n.Title,
		Body:              n.Body,
		Url:               n.URL,
		CreatedAt:         n.CreatedAt,
		ReadAt:            n.ReadAt,
		RespondedActionId: n.RespondedActionID,
		RespondedAt:       n.RespondedAt,
		RespondedVia:      n.RespondedVia,
	}
	if metadata != nil {
		result.Metadata = &metadata
	}
	if len(actions) > 0 {
		apiActions := make([]api.NotificationAction, 0, len(actions))
		for _, a := range actions {
			apiAction := api.NotificationAction{Id: a.ID, Label: a.Label}
			if a.Style != "" {
				style := a.Style
				apiAction.Style = &style
			}
			apiActions = append(apiActions, apiAction)
		}
		result.Actions = &apiActions
	}
	return result, nil
}

// requireSession pulls the authenticated session out of ctx — every
// notification endpoint that needs one is only ever reached once
// middleware.StrictSessionMiddleware has already required auth for its
// operationId, so a missing session here is always an internal
// inconsistency, not a real client error; still returned as 401 rather
// than panicking, on the same "fail safe" principle handlers/api_me.go's
// analogous check follows.
func requireSession(ctx context.Context) (*db.Session, bool) {
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return nil, false
	}
	return sessionObj, true
}

// CreateNotification handles POST /api/notifications — server-to-server,
// admin only (see AGENTS.md's notification package section for why this
// reuses the existing admin-owned API token mechanism rather than a new
// auth concept).
func (s *StrictApiServer) CreateNotification(ctx context.Context, request api.CreateNotificationRequestObject) (api.CreateNotificationResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.CreateNotification401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if !sessionObj.IsAdmin {
		return api.CreateNotification401JSONResponse{Code: http.StatusUnauthorized, Message: "Admin access required"}, nil
	}
	if s.notificationService == nil {
		return api.CreateNotification400JSONResponse{Code: http.StatusBadRequest, Message: "Notification service is not available"}, nil
	}
	if request.Body == nil {
		return api.CreateNotification400JSONResponse{Code: http.StatusBadRequest, Message: "Request body is required"}, nil
	}
	body := request.Body
	if len(body.UserIds) == 0 {
		return api.CreateNotification400JSONResponse{Code: http.StatusBadRequest, Message: "userIds must have at least one entry"}, nil
	}

	in := notification.CreateInput{
		UserIDs: body.UserIds,
		Type:    body.Type,
		Title:   body.Title,
		Body:    body.Body,
		URL:     body.Url,
	}
	if body.Metadata != nil {
		in.Metadata = *body.Metadata
	}
	if body.Actions != nil {
		for _, a := range *body.Actions {
			action := notification.Action{ID: a.Id, Label: a.Label}
			if a.Style != nil {
				action.Style = *a.Style
			}
			in.Actions = append(in.Actions, action)
		}
	}
	if body.Channels != nil {
		in.Channels = *body.Channels
	}

	notifications, err := s.notificationService.Create(ctx, in)
	if len(notifications) == 0 {
		// Every recipient failed to be stored — a genuine failure, not the
		// best-effort partial-success case below.
		if err != nil {
			return api.CreateNotification400JSONResponse{Code: http.StatusBadRequest, Message: err.Error()}, nil
		}
		return nil, fmt.Errorf("no notifications were created and no error was reported")
	}
	// Some recipients succeeded even if err is non-nil for the rest (see
	// Service.Create's doc comment) — respond with whatever did, the same
	// "don't lose the four that worked over the one that didn't"
	// philosophy delivery itself already follows. err is intentionally not
	// otherwise surfaced here; the caller can compare the returned
	// notifications' userIds against what it sent to see who was missed.
	apiNotifications := make([]api.Notification, 0, len(notifications))
	for _, n := range notifications {
		apiNotification, err := toAPINotification(n)
		if err != nil {
			return nil, err
		}
		apiNotifications = append(apiNotifications, apiNotification)
	}
	return api.CreateNotification201JSONResponse{Notifications: apiNotifications}, nil
}

// ListNotifications handles GET /api/notifications.
func (s *StrictApiServer) ListNotifications(ctx context.Context, request api.ListNotificationsRequestObject) (api.ListNotificationsResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.ListNotifications401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.notificationService == nil {
		return api.ListNotifications200JSONResponse{Notifications: []api.Notification{}}, nil
	}

	unreadOnly := request.Params.UnreadOnly != nil && *request.Params.UnreadOnly
	limit := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	notifications, err := s.notificationService.List(sessionObj.UserID, unreadOnly, limit, request.Params.Cursor)
	if err != nil {
		return nil, err
	}

	apiNotifications := make([]api.Notification, 0, len(notifications))
	for _, n := range notifications {
		apiNotification, err := toAPINotification(n)
		if err != nil {
			return nil, err
		}
		apiNotifications = append(apiNotifications, apiNotification)
	}

	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = notification.DefaultListLimit
	}
	response := api.NotificationListResponse{Notifications: apiNotifications}
	// A full page suggests there may be more — the cursor is simply the
	// last item's ID; a client that pages past the real end just gets an
	// empty page back (see NotificationRepositoryDB.ListNotifications).
	if len(notifications) == effectiveLimit {
		response.NextCursor = &notifications[len(notifications)-1].ID
	}
	return api.ListNotifications200JSONResponse(response), nil
}

// GetUnreadNotificationCount handles GET /api/notifications/unread-count.
func (s *StrictApiServer) GetUnreadNotificationCount(ctx context.Context, request api.GetUnreadNotificationCountRequestObject) (api.GetUnreadNotificationCountResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.GetUnreadNotificationCount401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.notificationService == nil {
		return api.GetUnreadNotificationCount200JSONResponse{Count: 0}, nil
	}
	count, err := s.notificationService.UnreadCount(sessionObj.UserID)
	if err != nil {
		return nil, err
	}
	return api.GetUnreadNotificationCount200JSONResponse{Count: int(count)}, nil
}

// MarkAllNotificationsRead handles POST /api/notifications/read-all.
func (s *StrictApiServer) MarkAllNotificationsRead(ctx context.Context, request api.MarkAllNotificationsReadRequestObject) (api.MarkAllNotificationsReadResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.MarkAllNotificationsRead401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.notificationService != nil {
		if err := s.notificationService.MarkAllRead(sessionObj.UserID); err != nil {
			return nil, err
		}
	}
	return api.MarkAllNotificationsRead204Response{}, nil
}

// MarkNotificationRead handles POST /api/notifications/{notificationId}/read.
func (s *StrictApiServer) MarkNotificationRead(ctx context.Context, request api.MarkNotificationReadRequestObject) (api.MarkNotificationReadResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.MarkNotificationRead401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.notificationService != nil {
		if err := s.notificationService.MarkRead(request.NotificationId, sessionObj.UserID); err != nil {
			return nil, err
		}
	}
	return api.MarkNotificationRead204Response{}, nil
}

// RespondToNotification handles POST /api/notifications/{notificationId}/respond
// — answering from the authenticated in-app list.
func (s *StrictApiServer) RespondToNotification(ctx context.Context, request api.RespondToNotificationRequestObject) (api.RespondToNotificationResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.RespondToNotification401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.notificationService == nil || request.Body == nil {
		return api.RespondToNotification404JSONResponse{Code: http.StatusNotFound, Message: "Notification not found"}, nil
	}

	_, err := s.notificationService.RespondViaWeb(ctx, request.NotificationId, sessionObj.UserID, request.Body.ActionId)
	switch {
	case err == nil:
		// fall through to fetch-and-return below
	case errors.Is(err, notification.ErrNotFound):
		return api.RespondToNotification404JSONResponse{Code: http.StatusNotFound, Message: "Notification not found"}, nil
	case errors.Is(err, notification.ErrForbidden):
		return api.RespondToNotification403JSONResponse{Code: http.StatusForbidden, Message: "This notification belongs to a different user"}, nil
	case errors.Is(err, notification.ErrInvalidAction):
		return api.RespondToNotification422JSONResponse{Code: 422, Message: "Not a valid action for this notification"}, nil
	case errors.Is(err, notification.ErrAlreadyResponded):
		return api.RespondToNotification409JSONResponse{Code: http.StatusConflict, Message: "This notification already has a recorded response"}, nil
	default:
		return nil, err
	}

	n, err := s.notificationService.Get(request.NotificationId, sessionObj.UserID)
	if err != nil {
		return api.RespondToNotification404JSONResponse{Code: http.StatusNotFound, Message: "Notification not found"}, nil
	}
	apiNotification, err := toAPINotification(n)
	if err != nil {
		return nil, err
	}
	return api.RespondToNotification200JSONResponse(apiNotification), nil
}

// respondHTMLPage renders a minimal, self-contained confirmation (or
// error) page for RespondToNotificationByToken — an email link opens
// directly in a browser, so this needs to be a readable page on its own,
// not a JSON blob.
func respondHTMLPage(heading, message string) api.RespondToNotificationByToken200TexthtmlResponse {
	html := fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;color:#1a1a1a}
h1{font-size:1.25rem}</style></head>
<body><h1>%s</h1><p>%s</p></body></html>`, heading, heading, message)
	return api.RespondToNotificationByToken200TexthtmlResponse{
		Body:          strings.NewReader(html),
		ContentLength: int64(len(html)),
	}
}

// RespondToNotificationByToken handles GET /api/notifications/respond —
// the public, unauthenticated email answer link. See its OpenAPI
// description for why this has no session/bearer security requirement
// (also listed in middleware.OperationWithNoSecurity).
func (s *StrictApiServer) RespondToNotificationByToken(ctx context.Context, request api.RespondToNotificationByTokenRequestObject) (api.RespondToNotificationByTokenResponseObject, error) {
	if s.notificationService == nil {
		return respondHTMLPage("Not available", "Notifications aren't available on this gateway right now."), nil
	}

	label, err := s.notificationService.RespondViaToken(ctx, request.Params.Token, request.Params.Action)
	switch {
	case err == nil:
		return respondHTMLPage("Thanks!", fmt.Sprintf("Your response (“%s”) has been recorded.", label)), nil
	case errors.Is(err, notification.ErrTokenExpired):
		return respondHTMLPage("Link expired", "This link has expired. Please check the app for the latest notifications."), nil
	case errors.Is(err, notification.ErrAlreadyResponded):
		return respondHTMLPage("Already responded", "A response to this notification has already been recorded."), nil
	case errors.Is(err, notification.ErrInvalidAction), errors.Is(err, notification.ErrNotFound):
		return respondHTMLPage("Link not valid", "This link is invalid. Please check the app for the latest notifications."), nil
	default:
		return nil, err
	}
}

// GetTelegramLinkCode handles GET /api/notifications/telegram/link.
func (s *StrictApiServer) GetTelegramLinkCode(ctx context.Context, request api.GetTelegramLinkCodeRequestObject) (api.GetTelegramLinkCodeResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.GetTelegramLinkCode401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.notificationService == nil {
		return api.GetTelegramLinkCode503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Telegram delivery isn't configured on this gateway"}, nil
	}
	deepLink, expiresAt, err := s.notificationService.GetTelegramLinkCode(ctx, sessionObj.UserID)
	if err != nil {
		if errors.Is(err, notification.ErrChannelNotConfigured) {
			return api.GetTelegramLinkCode503JSONResponse{Code: http.StatusServiceUnavailable, Message: "Telegram delivery isn't configured on this gateway"}, nil
		}
		return nil, err
	}
	return api.GetTelegramLinkCode200JSONResponse{DeepLink: deepLink, ExpiresAt: expiresAt}, nil
}

// toAPIDelivery converts one stored db.NotificationDelivery into the
// OpenAPI-generated shape. ExternalRef is deliberately not exposed — it's
// internal delivery plumbing (e.g. a Telegram "chatID:messageID" pair),
// not something an API caller needs.
func toAPIDelivery(d *db.NotificationDelivery) api.NotificationDelivery {
	result := api.NotificationDelivery{
		Id:            d.ID,
		Channel:       d.Channel,
		Status:        d.Status,
		AttemptNumber: d.AttemptNumber,
		CreatedAt:     d.CreatedAt,
		NextRetryAt:   d.NextRetryAt,
	}
	if d.Error != "" {
		result.Error = &d.Error
	}
	return result
}

// ListNotificationDeliveries handles GET /api/notifications/{notificationId}/deliveries.
func (s *StrictApiServer) ListNotificationDeliveries(ctx context.Context, request api.ListNotificationDeliveriesRequestObject) (api.ListNotificationDeliveriesResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.ListNotificationDeliveries401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.notificationService == nil {
		return api.ListNotificationDeliveries404JSONResponse{Code: http.StatusNotFound, Message: "Notification not found"}, nil
	}

	deliveries, err := s.notificationService.ListDeliveries(request.NotificationId, sessionObj.UserID, sessionObj.IsAdmin)
	switch {
	case err == nil:
		// fall through
	case errors.Is(err, notification.ErrNotFound):
		return api.ListNotificationDeliveries404JSONResponse{Code: http.StatusNotFound, Message: "Notification not found"}, nil
	case errors.Is(err, notification.ErrForbidden):
		return api.ListNotificationDeliveries403JSONResponse{Code: http.StatusForbidden, Message: "This notification belongs to a different user"}, nil
	default:
		return nil, err
	}

	apiDeliveries := make([]api.NotificationDelivery, 0, len(deliveries))
	for _, d := range deliveries {
		apiDeliveries = append(apiDeliveries, toAPIDelivery(d))
	}
	return api.ListNotificationDeliveries200JSONResponse{Deliveries: apiDeliveries}, nil
}

// GetNotificationPreference handles GET /api/notifications/preferences.
func (s *StrictApiServer) GetNotificationPreference(ctx context.Context, request api.GetNotificationPreferenceRequestObject) (api.GetNotificationPreferenceResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.GetNotificationPreference401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	if s.notificationService == nil {
		return api.GetNotificationPreference200JSONResponse{}, nil
	}
	channel, err := s.notificationService.GetPreferredChannel(sessionObj.UserID)
	if err != nil {
		return nil, err
	}
	return api.GetNotificationPreference200JSONResponse{PreferredChannel: emptyToNil(channel)}, nil
}

// SetNotificationPreference handles PUT /api/notifications/preferences.
func (s *StrictApiServer) SetNotificationPreference(ctx context.Context, request api.SetNotificationPreferenceRequestObject) (api.SetNotificationPreferenceResponseObject, error) {
	sessionObj, ok := requireSession(ctx)
	if !ok {
		return api.SetNotificationPreference401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized"}, nil
	}
	var channel string
	if request.Body != nil && request.Body.PreferredChannel != nil {
		channel = *request.Body.PreferredChannel
	}
	if s.notificationService != nil {
		if err := s.notificationService.SetPreferredChannel(sessionObj.UserID, channel); err != nil {
			return nil, err
		}
	}
	return api.SetNotificationPreference200JSONResponse{PreferredChannel: emptyToNil(channel)}, nil
}

// emptyToNil returns nil for an empty string, or a pointer to s otherwise —
// api.NotificationPreference.PreferredChannel is optional/nullable, and an
// empty string on the wire is indistinguishable from "no preference set"
// either way, so there's no reason to send an explicit "" over omitting
// the field entirely.
func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
