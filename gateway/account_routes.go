package gateway

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/providers"
)

// authPageNames are the account page templates, each rendered inside
// auth_layout.html.
var authPageNames = []string{"message.html", "account.html", "notification.html"}

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

// authPage is the data every account page template gets.
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
	Account   *accountView
	// Notification is the page an SMS/WhatsApp link opens.
	Notification *notificationView
}

type accountView struct {
	Email          string
	Name           string
	EmailConfirmed bool
	HasPassword    bool
	CSRF           string
	Identities     []identityView
	Linkable       []linkableProvider
	Sessions       []sessionView
	Events         []eventView
}

type identityView struct {
	ID, ProviderLabel, Email, LastLogin string
}

type linkableProvider struct {
	Label, LinkPath string
}

type sessionView struct {
	Hash, Device, IP, Location, LastActive string
	Current                                bool
}

type eventView struct {
	Label, When, IP string
}

// Messages shown for ?msg= and ?error= codes on the login and account pages.
var (
	pageMessages = map[string]string{
		"linked":           "The account was linked. You can now log in with it too.",
		"unlinked":         "The account was unlinked.",
		"password_changed": "Your password was changed. Other sessions were signed out.",
		"session_revoked":  "The session was signed out.",
		"sessions_revoked": "All other sessions were signed out.",
	}
	pageErrors = map[string]string{
		"link_required":      accounts.ErrLinkRequired.Error(),
		"email_not_verified": accounts.ErrEmailNotVerified.Error() + ". Verify it with the provider, or use another way to log in.",
		"identity_taken":     accounts.ErrIdentityTaken.Error() + ".",
	}
)

var providerLabels = map[string]string{
	"google": "Google",
	"github": "GitHub",
	"basic":  "Password",
}

var eventLabels = map[string]string{
	db.AuthEventLogin:            "Logged in",
	db.AuthEventLoginFailed:      "Failed login (wrong password)",
	db.AuthEventSignup:           "Account created",
	db.AuthEventPasswordChanged:  "Password changed",
	db.AuthEventIdentityLinked:   "Account linked",
	db.AuthEventIdentityUnlinked: "Account unlinked",
	db.AuthEventSessionRevoked:   "Session signed out",
}

func labelOr(labels map[string]string, key string) string {
	if label, ok := labels[key]; ok {
		return label
	}
	return key
}

// providerLabel names a login method for people: built-in ones from
// providerLabels, OIDC providers by their configured label.
func (g *Gateway) providerLabel(name string) string {
	for _, p := range g.GatewayConfig.AuthenticationProviders.OIDC {
		if p.Name == name {
			return p.DisplayLabel()
		}
	}
	return labelOr(providerLabels, name)
}

func (g *Gateway) newAuthPage(title string) authPage {
	return authPage{Title: title, Prefix: config.ManagementPrefix, LogoURL: g.GatewayConfig.Branding.LogoUrl, Version: DisplayVersion(g.Version)}
}

func (g *Gateway) renderAuthPage(w http.ResponseWriter, status int, name string, page authPage) {
	tmpl, ok := g.authTemplates[name]
	if !ok {
		log.Printf("Error: account template '%s' not found", name)
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

// registerAccountRoutes registers the account page and its actions.
func (g *Gateway) registerAccountRoutes() {
	if !g.GatewayConfig.HasAnyAuthentication() {
		return
	}
	g.Mux.HandleFunc("GET "+providers.AccountPath, g.handleAccountPage)
	g.Mux.HandleFunc("POST "+providers.AccountPath+"/password", g.withAccount(g.handleChangePassword))
	g.Mux.HandleFunc("POST "+providers.AccountPath+"/sessions/revoke", g.withAccount(g.handleRevokeSession))
	g.Mux.HandleFunc("POST "+providers.AccountPath+"/sessions/revoke-others", g.withAccount(g.handleRevokeOtherSessions))
	g.Mux.HandleFunc("POST "+providers.AccountPath+"/identities/unlink", g.withAccount(g.handleUnlinkIdentity))
	log.Printf("Registered account routes")
}

// --- Account page ---

// accountRequest is a signed-in user on the account page.
type accountRequest struct {
	session *db.Session
	user    *db.User
}

// currentAccount returns the signed-in user, or false.
func (g *Gateway) currentAccount(r *http.Request) (*accountRequest, bool) {
	sessionObj, ok := g.Dependencies.SessionStore.ValidateSession(r)
	if !ok || sessionObj == nil || sessionObj.UserID == "" {
		return nil, false
	}
	user, err := g.Dependencies.UserRepo.FindUserByIdOrUsername(sessionObj.UserID, "", "")
	if err != nil {
		return nil, false
	}
	return &accountRequest{session: sessionObj, user: user}, true
}

// withAccount guards an account form POST: a signed-in user and the CSRF
// token of their session.
func (g *Gateway) withAccount(next func(http.ResponseWriter, *http.Request, *accountRequest)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct, ok := g.currentAccount(r)
		if !ok {
			http.Redirect(w, r, config.ManagementPrefix+"/login?redirect="+providers.AccountPath, http.StatusFound)
			return
		}
		if !accounts.ValidCSRF(acct.session.Token, r.FormValue("csrf")) {
			http.Error(w, "Forbidden: the form expired, reload the page and try again", http.StatusForbidden)
			return
		}
		next(w, r, acct)
	}
}

func accountRedirect(w http.ResponseWriter, r *http.Request, key, code string) {
	http.Redirect(w, r, providers.AccountPath+"?"+key+"="+code, http.StatusFound)
}

func (g *Gateway) handleAccountPage(w http.ResponseWriter, r *http.Request) {
	acct, ok := g.currentAccount(r)
	if !ok {
		http.Redirect(w, r, config.ManagementPrefix+"/login?redirect="+providers.AccountPath, http.StatusFound)
		return
	}
	g.renderAccountPage(w, http.StatusOK, acct, pageMessages[r.URL.Query().Get("msg")], pageErrors[r.URL.Query().Get("error")])
}

func (g *Gateway) renderAccountPage(w http.ResponseWriter, status int, acct *accountRequest, message, errorText string) {
	user := acct.user
	view := &accountView{
		Email:          user.Email,
		Name:           user.Name,
		EmailConfirmed: user.EmailConfirmed,
		HasPassword:    user.Password != "",
		CSRF:           accounts.CSRFToken(acct.session.Token),
	}

	identities, err := g.Accounts.Identities(user.ID)
	if err != nil {
		log.Printf("Account page: listing identities of %s failed: %v", user.ID, err)
	}
	linked := map[string]bool{}
	for _, id := range identities {
		linked[id.Provider] = true
		lastLogin := "never"
		if id.LastLoginAt != nil {
			lastLogin = id.LastLoginAt.Local().Format("2006-01-02 15:04")
		}
		view.Identities = append(view.Identities, identityView{ID: id.ID, ProviderLabel: g.providerLabel(id.Provider), Email: id.Email, LastLogin: lastLogin})
	}
	providerCfg := g.GatewayConfig.AuthenticationProviders
	for _, p := range []struct {
		name  string
		creds config.AuthProviderCredentials
	}{{"google", providerCfg.Google}, {"github", providerCfg.Github}} {
		if p.creds.ClientId != "" && p.creds.ClientSecret != "" && !linked[p.name] {
			view.Linkable = append(view.Linkable, linkableProvider{Label: providerLabels[p.name], LinkPath: config.ManagementPrefix + "/auth/" + p.name + "/link"})
		}
	}
	for _, p := range providerCfg.OIDC {
		if !linked[p.Name] {
			view.Linkable = append(view.Linkable, linkableProvider{Label: p.DisplayLabel(), LinkPath: config.ManagementPrefix + "/auth/" + p.Name + "/link"})
		}
	}

	sessions, err := g.Accounts.Sessions(user.ID)
	if err != nil {
		log.Printf("Account page: listing sessions of %s failed: %v", user.ID, err)
	}
	currentHash := db.HashSessionToken(acct.session.Token)
	for _, s := range sessions {
		view.Sessions = append(view.Sessions, sessionView{
			Hash:       s.TokenHash,
			Device:     describeDevice(s.ClientInfo),
			IP:         s.IPAddress,
			Location:   joinNonEmpty(", ", s.City, s.Country),
			LastActive: s.LastActivity.Local().Format("2006-01-02 15:04"),
			Current:    s.TokenHash == currentHash,
		})
	}

	events, err := g.Accounts.RecentEvents(user.ID, 20)
	if err != nil {
		log.Printf("Account page: listing activity of %s failed: %v", user.ID, err)
	}
	for _, e := range events {
		label := labelOr(eventLabels, e.Type)
		if e.Provider != "" {
			label += " · " + g.providerLabel(e.Provider)
		}
		view.Events = append(view.Events, eventView{Label: label, When: e.CreatedAt.Local().Format("2006-01-02 15:04"), IP: e.IPAddress})
	}

	page := g.newAuthPage("Your account")
	page.Wide = true
	page.Message = message
	page.Error = errorText
	page.Account = view
	g.renderAuthPage(w, status, "account.html", page)
}

func describeDevice(c db.ClientInfo) string {
	browser := c.BrowserFamily
	if browser == "" || browser == "Other" {
		browser = "Unknown browser"
	}
	if c.OSFamily != "" && c.OSFamily != "Other" {
		return browser + " on " + c.OSFamily
	}
	return browser
}

func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

func (g *Gateway) handleChangePassword(w http.ResponseWriter, r *http.Request, acct *accountRequest) {
	err := g.Accounts.ChangePassword(acct.user, r.FormValue("current_password"), r.FormValue("new_password"),
		db.HashSessionToken(acct.session.Token), accounts.ClientFromRequest(r))
	if errors.Is(err, accounts.ErrWrongPassword) || errors.Is(err, accounts.ErrWeakPassword) || errors.Is(err, accounts.ErrLongPassword) {
		g.renderAccountPage(w, http.StatusBadRequest, acct, "", err.Error()+".")
		return
	}
	if err != nil {
		log.Printf("Password change for %s failed: %v", acct.user.ID, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	accountRedirect(w, r, "msg", "password_changed")
}

func (g *Gateway) handleRevokeSession(w http.ResponseWriter, r *http.Request, acct *accountRequest) {
	hash := r.FormValue("session")
	err := g.Accounts.RevokeSession(acct.user.ID, hash, accounts.ClientFromRequest(r))
	if err != nil {
		g.renderAccountPage(w, http.StatusBadRequest, acct, "", "That session is already signed out.")
		return
	}
	if hash == db.HashSessionToken(acct.session.Token) {
		http.Redirect(w, r, config.ManagementPrefix+"/login", http.StatusFound)
		return
	}
	accountRedirect(w, r, "msg", "session_revoked")
}

func (g *Gateway) handleRevokeOtherSessions(w http.ResponseWriter, r *http.Request, acct *accountRequest) {
	_, err := g.Accounts.RevokeOtherSessions(acct.user.ID, db.HashSessionToken(acct.session.Token), accounts.ClientFromRequest(r))
	if err != nil {
		log.Printf("Revoking other sessions of %s failed: %v", acct.user.ID, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	accountRedirect(w, r, "msg", "sessions_revoked")
}

func (g *Gateway) handleUnlinkIdentity(w http.ResponseWriter, r *http.Request, acct *accountRequest) {
	err := g.Accounts.UnlinkIdentity(acct.user, r.FormValue("id"), accounts.ClientFromRequest(r))
	if errors.Is(err, accounts.ErrLastLoginMethod) {
		g.renderAccountPage(w, http.StatusBadRequest, acct, "", err.Error()+".")
		return
	}
	if err != nil {
		g.renderAccountPage(w, http.StatusBadRequest, acct, "", "That account is not linked.")
		return
	}
	accountRedirect(w, r, "msg", "unlinked")
}
