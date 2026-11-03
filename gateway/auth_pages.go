package gateway

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/jmaister/taronja-gateway/config"
)

// Version is the build version shown in the page footer; the CLI sets it at startup.
var Version = "Dev"

func footerVersion() string {
	if Version != "" && Version[0] >= '0' && Version[0] <= '9' {
		return "v" + Version
	}
	return Version
}

// authPageNames are the server-rendered page templates, each rendered inside
// auth_layout.html.
var authPageNames = []string{
	"message.html", "account.html", "pay.html", "pay_done.html",
}

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
	Brand   string
	Message string
	Error   string
	Wide    bool
	Version string

	// ButtonColor and BackgroundColor (hex) come from billing.brands.
	ButtonColor, BackgroundColor, OnPrimary string

	Token     string
	Body      string
	LinkURL   string
	LinkLabel string
	Account   *accountView
	Pay       *payView
	PayDone   *payDoneView
}

func (g *Gateway) newAuthPage(title string) authPage {
	brand := g.GatewayConfig.Billing.Email.BrandName
	if brand == "" {
		brand = g.GatewayConfig.Name
	}
	return authPage{Title: title, Prefix: config.ManagementPrefix, LogoURL: g.GatewayConfig.Branding.LogoUrl, Brand: brand, Version: footerVersion()}
}

// brandAuthPage applies the brand of a plan group to page: its name and logo,
// and its colors, over the gateway's.
func brandAuthPage(page authPage, b billing.Brand) authPage {
	if b.Name != "" {
		page.Brand = b.Name
	}
	if b.LogoURL != "" {
		page.LogoURL = b.LogoURL
	}
	page.ButtonColor, page.BackgroundColor = b.ButtonColor, b.BackgroundColor
	if b.ButtonColor != "" {
		page.OnPrimary = textColorOn(b.ButtonColor)
	}
	return page
}

// textColorOn is white or near-black, whichever reads better on the hex color bg.
func textColorOn(bg string) string {
	var r, g, b int
	_, err := fmt.Sscanf(bg, "#%02x%02x%02x", &r, &g, &b)
	if err != nil || 299*r+587*g+114*b >= 150000 {
		return "#0b1020"
	}
	return "#fff"
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
