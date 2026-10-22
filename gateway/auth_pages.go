package gateway

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
)

// authPageNames are the server-rendered page templates, each rendered inside
// auth_layout.html.
var authPageNames = []string{"message.html", "notification.html"}

func parseAuthTemplates(fs embed.FS) (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template, len(authPageNames))
	for _, name := range authPageNames {
		t, err := template.ParseFS(fs, "auth_layout.html", name)
		if err != nil {
			return nil, fmt.Errorf("error parsing template '%s': %w", name, err)
		}
		templates[name] = t
	}
	return templates, nil
}

// authPage is the data every server-rendered page template gets.
type authPage struct {
	Title   string
	Prefix  string
	LogoURL string
	Message string
	Error   string
	Wide    bool
	Version string // footer

	Token     string
	Body      string
	LinkURL   string
	LinkLabel string
	// Notification is the page an SMS/WhatsApp link opens.
	Notification *notificationView
}

func (g *Gateway) newAuthPage(title string) authPage {
	return authPage{Title: title, Prefix: config.ManagementPrefix, LogoURL: g.GatewayConfig.Branding.LogoUrl, Version: DisplayVersion(g.Version)}
}

func (g *Gateway) renderAuthPage(w http.ResponseWriter, status int, name string, page authPage) {
	tmpl, ok := g.authTemplates[name]
	if !ok {
		log.Printf("Error: page template '%s' not found", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	err := tmpl.ExecuteTemplate(w, "layout", page)
	if err != nil {
		log.Printf("Error rendering %s: %v", name, err)
	}
}

func (g *Gateway) renderMessage(w http.ResponseWriter, status int, title, body, linkURL, linkLabel string) {
	page := g.newAuthPage(title)
	page.Body = body
	page.LinkURL = linkURL
	page.LinkLabel = linkLabel
	g.renderAuthPage(w, status, "message.html", page)
}
