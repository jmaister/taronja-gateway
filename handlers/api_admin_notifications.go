package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/notification"
)

// ListAdminNotifications handles GET /api/admin/notifications.
func (s *StrictApiServer) ListAdminNotifications(ctx context.Context, request api.ListAdminNotificationsRequestObject) (api.ListAdminNotificationsResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.ListAdminNotifications401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized: Admin access required"}, nil
	}
	p := request.Params
	page, limit := 1, notification.DefaultAdminPageSize
	if p.Page != nil {
		page = *p.Page
	}
	if p.Limit != nil {
		limit = *p.Limit
	}
	if page < 1 || limit < 1 || limit > notification.MaxAdminPageSize {
		return api.ListAdminNotifications400JSONResponse{Code: http.StatusBadRequest, Message: "page must be at least 1 and limit between 1 and 200"}, nil
	}

	filter := repositories.AdminNotificationFilter{
		From:      (*time.Time)(p.StartDate),
		To:        (*time.Time)(p.EndDate),
		Type:      deref(p.Type),
		UserID:    deref(p.UserId),
		User:      deref(p.User),
		BatchID:   deref(p.BatchId),
		Channel:   deref(p.Channel),
		Responded: p.Responded,
		Read:      p.Read,
		Search:    deref(p.Q),
	}
	if p.Status != nil {
		filter.Status = string(*p.Status)
		if !repositories.IsAdminStatus(filter.Status) {
			return api.ListAdminNotifications400JSONResponse{Code: http.StatusBadRequest, Message: "unknown status " + filter.Status}, nil
		}
	}
	if filter.From != nil && filter.To != nil && filter.To.Before(*filter.From) {
		return api.ListAdminNotifications400JSONResponse{Code: http.StatusBadRequest, Message: "end_date is before start_date"}, nil
	}
	if s.notificationService == nil {
		return api.ListAdminNotifications200JSONResponse{Items: []api.AdminNotification{}, Page: page, Limit: limit}, nil
	}

	rows, total, err := s.notificationService.AdminList(filter, page, limit)
	if err != nil {
		log.Printf("ListAdminNotifications: %v", err)
		return api.ListAdminNotifications500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list notifications"}, nil
	}
	items := make([]api.AdminNotification, 0, len(rows))
	for i := range rows {
		n, err := toAPINotification(&rows[i].Notification)
		if err != nil {
			log.Printf("ListAdminNotifications: %v", err)
			return api.ListAdminNotifications500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list notifications"}, nil
		}
		items = append(items, api.AdminNotification{
			Notification: n,
			Username:     rows[i].Username,
			Email:        rows[i].Email,
			Status:       api.AdminNotificationStatus(rows[i].Status),
			Channels:     rows[i].Channels,
		})
	}
	return api.ListAdminNotifications200JSONResponse{Items: items, Total: int(total), Page: page, Limit: limit}, nil
}

// ListNotificationTypes handles GET /api/admin/notifications/types.
func (s *StrictApiServer) ListNotificationTypes(ctx context.Context, request api.ListNotificationTypesRequestObject) (api.ListNotificationTypesResponseObject, error) {
	if _, ok := requireAdminSession(ctx); !ok {
		return api.ListNotificationTypes401JSONResponse{Code: http.StatusUnauthorized, Message: "Unauthorized: Admin access required"}, nil
	}
	if s.notificationService == nil {
		return api.ListNotificationTypes200JSONResponse{Types: []string{}}, nil
	}
	types, err := s.notificationService.NotificationTypes()
	if err != nil {
		log.Printf("ListNotificationTypes: %v", err)
		return api.ListNotificationTypes500JSONResponse{Code: http.StatusInternalServerError, Message: "Failed to list notification types"}, nil
	}
	return api.ListNotificationTypes200JSONResponse{Types: types}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
