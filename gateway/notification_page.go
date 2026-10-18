package gateway

import (
	"errors"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/notification"
)

type notificationView struct {
	Body     string
	URL      string
	Actions  []notification.Action
	Answered string // label of the chosen action, once answered
}

// registerNotificationPage serves the page SMS and WhatsApp links open
// (/_/n/<token>): the notification, its URL, and a button per action. The
// token is the credential, as with the email answer links.
func (g *Gateway) registerNotificationPage() {
	if g.Dependencies.NotificationService == nil {
		return
	}
	path := config.ManagementPrefix + "/n/{token}"
	g.Mux.HandleFunc("GET "+path, g.handleNotificationPage)
	g.Mux.HandleFunc("POST "+path, g.handleNotificationAnswer)
}

func (g *Gateway) handleNotificationPage(w http.ResponseWriter, r *http.Request) {
	g.renderNotificationPage(w, r.PathValue("token"), "", "")
}

func (g *Gateway) handleNotificationAnswer(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	label, err := g.Dependencies.NotificationService.RespondViaLink(r.Context(), token, r.FormValue("action"))
	switch {
	case err == nil:
		g.renderNotificationPage(w, token, "Thanks, your answer was recorded: "+label+".", "")
	case errors.Is(err, notification.ErrAlreadyResponded):
		g.renderNotificationPage(w, token, "", "This notification was already answered.")
	case errors.Is(err, notification.ErrInvalidAction):
		g.renderNotificationPage(w, token, "", "That is not one of the answers.")
	default:
		g.renderNotificationPage(w, token, "", "")
	}
}

func (g *Gateway) renderNotificationPage(w http.ResponseWriter, token, message, errorText string) {
	n, actions, err := g.Dependencies.NotificationService.NotificationPage(token)
	switch {
	case errors.Is(err, notification.ErrTokenExpired):
		g.renderMessage(w, http.StatusGone, "Link expired", "This link has expired.", "", "")
		return
	case err != nil:
		if !errors.Is(err, notification.ErrNotFound) {
			log.Printf("Notification page: %v", err)
		}
		g.renderMessage(w, http.StatusNotFound, "Link not valid", "This link is not valid.", "", "")
		return
	}

	view := &notificationView{Body: n.Body, Actions: actions}
	if n.URL != nil {
		view.URL = *n.URL
	}
	if n.RespondedActionID != nil {
		view.Answered = *n.RespondedActionID
		for _, a := range actions {
			if a.ID == *n.RespondedActionID {
				view.Answered = a.Label
			}
		}
	}
	page := g.newAuthPage(n.Title)
	page.Token = token
	page.Message = message
	page.Error = errorText
	page.Notification = view
	g.renderAuthPage(w, http.StatusOK, "notification.html", page)
}
