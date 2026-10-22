package gateway

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/providers"
	"github.com/jmaister/taronja-gateway/session"
)

// emailLoginCookieName holds the browser binding of a pending email login: the
// emailed code is only accepted from the browser that asked for it.
const emailLoginCookieName = "tg_email_login"

// authPageNames are the account page templates, each rendered inside
// auth_layout.html.
var authPageNames = []string{
	"signup.html", "forgot_password.html", "reset_password.html",
	"email_code.html", "confirm_link.html", "message.html", "account.html",
	"notification.html",
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

// authPage is the data every account page template gets.
type authPage struct {
	Title   string
	Prefix  string
	LogoURL string
	Message string
	Error   string
	Wide    bool
	Version string // footer

	Email       string
	Redirect    string
	Token       string
	Body        string
	Action      string
	ButtonLabel string
	LinkURL     string
	LinkLabel   string
	Account     *accountView
	// Notification is the page an SMS/WhatsApp link opens.
	Notification *notificationView
}

type accountView struct {
	Email           string
	Name            string
	EmailConfirmed  bool
	CanConfirmEmail bool
	HasPassword     bool
	CSRF            string
	Identities      []identityView
	Linkable        []linkableProvider
	Sessions        []sessionView
	Events          []eventView
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
		"password_reset":   "Your password was reset and you are logged in. Other sessions were signed out.",
		"session_revoked":  "The session was signed out.",
		"sessions_revoked": "All other sessions were signed out.",
		"email_sent":       "We sent you a confirmation email.",
		"email_verified":   "Your email is confirmed.",
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
	"email":  "Email link",
}

var eventLabels = map[string]string{
	db.AuthEventLogin:                  "Logged in",
	db.AuthEventLoginFailed:            "Failed login (wrong password)",
	db.AuthEventSignup:                 "Account created",
	db.AuthEventEmailVerified:          "Email confirmed",
	db.AuthEventPasswordResetRequested: "Password reset requested",
	db.AuthEventPasswordReset:          "Password reset",
	db.AuthEventPasswordChanged:        "Password changed",
	db.AuthEventEmailLoginRequested:    "Login link requested",
	db.AuthEventIdentityLinked:         "Account linked",
	db.AuthEventIdentityUnlinked:       "Account unlinked",
	db.AuthEventSessionRevoked:         "Session signed out",
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

// startSessionAndRedirect logs user in and sends them to redirect.
func (g *Gateway) startSessionAndRedirect(w http.ResponseWriter, r *http.Request, user *db.User, providerName, redirect string) {
	_, err := providers.StartSession(w, r, user, providerName, g.Dependencies.SessionStore, g.GatewayConfig)
	if err != nil {
		log.Printf("Error creating session for %s: %v", user.ID, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, session.SanitizeRedirectPath(redirect), http.StatusFound)
}

// registerAccountRoutes registers sign-up, email verification, password
// reset, email login and the account page. Each flow is only registered when
// the config enables it.
func (g *Gateway) registerAccountRoutes() {
	prefix := config.ManagementPrefix
	opts := g.Accounts.Options()

	if opts.SignupEnabled {
		g.Mux.HandleFunc("GET "+prefix+"/signup", g.handleSignupForm)
		g.Mux.HandleFunc("POST "+prefix+"/signup", g.handleSignup)
	}
	if opts.SignupEnabled || g.GatewayConfig.Notification.Email.IsConfigured() {
		g.Mux.HandleFunc("GET "+accounts.VerifyEmailPath, g.handleConfirmLinkPage(accounts.VerifyEmailPath, "Confirm your email", "Confirm your email address to finish.", "Confirm email"))
		g.Mux.HandleFunc("POST "+accounts.VerifyEmailPath, g.handleVerifyEmail)
	}
	if opts.PasswordResetEnabled {
		g.Mux.HandleFunc("GET "+accounts.ForgotPasswordPath, g.handleForgotPasswordForm)
		g.Mux.HandleFunc("POST "+accounts.ForgotPasswordPath, g.handleForgotPassword)
		g.Mux.HandleFunc("GET "+accounts.ResetPasswordPath, g.handleResetPasswordForm)
		g.Mux.HandleFunc("POST "+accounts.ResetPasswordPath, g.handleResetPassword)
	}
	if opts.EmailLoginEnabled {
		g.Mux.HandleFunc("POST "+prefix+"/auth/email/start", g.handleEmailLoginStart)
		g.Mux.HandleFunc("POST "+prefix+"/auth/email/code", g.handleEmailLoginCode)
		g.Mux.HandleFunc("GET "+accounts.EmailLoginLinkPath, g.handleConfirmLinkPage(accounts.EmailLoginLinkPath, "Log in", "Continue to log in with your email link.", "Log in"))
		g.Mux.HandleFunc("POST "+accounts.EmailLoginLinkPath, g.handleEmailLoginLink)
	}

	if g.GatewayConfig.HasAnyAuthentication() {
		g.Mux.HandleFunc("GET "+providers.AccountPath, g.handleAccountPage)
		g.Mux.HandleFunc("POST "+providers.AccountPath+"/password", g.withAccount(g.handleChangePassword))
		g.Mux.HandleFunc("POST "+providers.AccountPath+"/sessions/revoke", g.withAccount(g.handleRevokeSession))
		g.Mux.HandleFunc("POST "+providers.AccountPath+"/sessions/revoke-others", g.withAccount(g.handleRevokeOtherSessions))
		g.Mux.HandleFunc("POST "+providers.AccountPath+"/identities/unlink", g.withAccount(g.handleUnlinkIdentity))
		g.Mux.HandleFunc("POST "+providers.AccountPath+"/verify-email", g.withAccount(g.handleSendEmailConfirmation))
	}
	log.Printf("Registered account routes (signup: %t, password reset: %t, email login: %t)", opts.SignupEnabled, opts.PasswordResetEnabled, opts.EmailLoginEnabled)
}

// --- Sign-up and email verification ---

func (g *Gateway) handleSignupForm(w http.ResponseWriter, r *http.Request) {
	page := g.newAuthPage("Create an account")
	page.Redirect = session.SanitizeRedirectPath(r.URL.Query().Get("redirect"))
	g.renderAuthPage(w, http.StatusOK, "signup.html", page)
}

func (g *Gateway) handleSignup(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	err := g.Accounts.SignUp(r.Context(), email, r.FormValue("password"), r.FormValue("redirect"))
	if errors.Is(err, accounts.ErrInvalidEmail) || errors.Is(err, accounts.ErrWeakPassword) || errors.Is(err, accounts.ErrLongPassword) {
		page := g.newAuthPage("Create an account")
		page.Error = err.Error()
		page.Email = email
		page.Redirect = session.SanitizeRedirectPath(r.FormValue("redirect"))
		g.renderAuthPage(w, http.StatusBadRequest, "signup.html", page)
		return
	}
	if err != nil && !errors.Is(err, accounts.ErrTooSoon) {
		log.Printf("Sign-up failed: %v", err)
		g.renderMessage(w, http.StatusInternalServerError, "Something went wrong", "We could not send the confirmation email. Try again later.", config.ManagementPrefix+"/signup", "Back")
		return
	}
	g.renderMessage(w, http.StatusOK, "Check your inbox",
		fmt.Sprintf("We sent an email to %s. Open the link in it to finish creating your account.", strings.TrimSpace(email)), "", "")
}

// handleConfirmLinkPage shows a button that POSTs the emailed token. Using a
// token on GET would let mail scanners that prefetch links burn it.
func (g *Gateway) handleConfirmLinkPage(action, title, body, button string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := g.newAuthPage(title)
		page.Body = body
		page.Action = action
		page.ButtonLabel = button
		page.Token = r.URL.Query().Get("token")
		g.renderAuthPage(w, http.StatusOK, "confirm_link.html", page)
	}
}

func (g *Gateway) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	user, redirect, err := g.Accounts.VerifyEmail(r.FormValue("token"), accounts.ClientFromRequest(r))
	switch {
	case errors.Is(err, accounts.ErrInvalidToken):
		g.renderMessage(w, http.StatusBadRequest, "Link not valid", err.Error()+".", config.ManagementPrefix+"/login", "Go to login")
		return
	case errors.Is(err, accounts.ErrAccountExists):
		g.renderMessage(w, http.StatusConflict, "Account already exists", err.Error()+".", config.ManagementPrefix+"/login", "Go to login")
		return
	case err != nil:
		log.Printf("Email verification failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if redirect == providers.AccountPath {
		redirect += "?msg=email_verified"
	}
	g.startSessionAndRedirect(w, r, user, user.Provider, redirect)
}

// --- Password reset ---

func (g *Gateway) handleForgotPasswordForm(w http.ResponseWriter, r *http.Request) {
	g.renderAuthPage(w, http.StatusOK, "forgot_password.html", g.newAuthPage("Reset your password"))
}

func (g *Gateway) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	err := g.Accounts.RequestPasswordReset(r.Context(), email, accounts.ClientFromRequest(r))
	if errors.Is(err, accounts.ErrInvalidEmail) {
		page := g.newAuthPage("Reset your password")
		page.Error = err.Error()
		page.Email = email
		g.renderAuthPage(w, http.StatusBadRequest, "forgot_password.html", page)
		return
	}
	if err != nil {
		log.Printf("Password reset request failed: %v", err)
	}
	// The same answer whether or not the account exists.
	g.renderMessage(w, http.StatusOK, "Check your inbox",
		fmt.Sprintf("If %s has an account, we sent it a link to choose a new password. The link expires in 1 hour.", strings.TrimSpace(email)),
		config.ManagementPrefix+"/login", "Back to login")
}

func (g *Gateway) handleResetPasswordForm(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	err := g.Accounts.CheckPasswordResetToken(token)
	if err != nil {
		g.renderMessage(w, http.StatusBadRequest, "Link not valid", accounts.ErrInvalidToken.Error()+".", accounts.ForgotPasswordPath, "Ask for a new link")
		return
	}
	page := g.newAuthPage("Choose a new password")
	page.Token = token
	g.renderAuthPage(w, http.StatusOK, "reset_password.html", page)
}

func (g *Gateway) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	user, err := g.Accounts.ResetPassword(token, r.FormValue("password"), accounts.ClientFromRequest(r))
	switch {
	case errors.Is(err, accounts.ErrWeakPassword), errors.Is(err, accounts.ErrLongPassword):
		page := g.newAuthPage("Choose a new password")
		page.Token = token
		page.Error = err.Error()
		g.renderAuthPage(w, http.StatusBadRequest, "reset_password.html", page)
		return
	case errors.Is(err, accounts.ErrInvalidToken):
		g.renderMessage(w, http.StatusBadRequest, "Link not valid", err.Error()+".", accounts.ForgotPasswordPath, "Ask for a new link")
		return
	case err != nil:
		log.Printf("Password reset failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	g.startSessionAndRedirect(w, r, user, "basic", providers.AccountPath+"?msg=password_reset")
}

// --- Email login ---

func (g *Gateway) handleEmailLoginStart(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	binding, err := g.Accounts.StartEmailLogin(r.Context(), email, r.FormValue("redirect"), accounts.ClientFromRequest(r))
	page := g.newAuthPage("Check your inbox")
	page.Email = strings.TrimSpace(email)
	switch {
	case errors.Is(err, accounts.ErrInvalidEmail):
		g.redirectToLogin(w, r, err.Error())
		return
	case errors.Is(err, accounts.ErrTooSoon):
		// Keep the binding from the first request: its code is still valid.
		page.Message = err.Error() + "."
		g.renderAuthPage(w, http.StatusTooManyRequests, "email_code.html", page)
		return
	case err != nil:
		log.Printf("Email login start failed: %v", err)
		g.renderMessage(w, http.StatusInternalServerError, "Something went wrong", "We could not send the login email. Try again later.", config.ManagementPrefix+"/login", "Back to login")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     emailLoginCookieName,
		Value:    binding,
		Path:     config.ManagementPrefix + "/auth/email/",
		HttpOnly: true,
		Secure:   session.RequestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((15 * time.Minute).Seconds()),
	})
	g.renderAuthPage(w, http.StatusOK, "email_code.html", page)
}

func (g *Gateway) handleEmailLoginCode(w http.ResponseWriter, r *http.Request) {
	var binding string
	cookie, err := r.Cookie(emailLoginCookieName)
	if err == nil {
		binding = cookie.Value
	}
	user, redirect, err := g.Accounts.FinishEmailLoginByCode(binding, r.FormValue("code"), accounts.ClientFromRequest(r))
	switch {
	case errors.Is(err, accounts.ErrInvalidCode):
		page := g.newAuthPage("Check your inbox")
		page.Error = err.Error() + ". Try again."
		g.renderAuthPage(w, http.StatusBadRequest, "email_code.html", page)
		return
	case errors.Is(err, accounts.ErrTooManyAttempts), errors.Is(err, accounts.ErrInvalidToken):
		g.redirectToLogin(w, r, err.Error()+".")
		return
	case err != nil:
		log.Printf("Email login by code failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: emailLoginCookieName, Value: "", Path: config.ManagementPrefix + "/auth/email/", HttpOnly: true, Secure: session.RequestIsSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	g.startSessionAndRedirect(w, r, user, "email", redirect)
}

func (g *Gateway) handleEmailLoginLink(w http.ResponseWriter, r *http.Request) {
	user, redirect, err := g.Accounts.FinishEmailLoginByLink(r.FormValue("token"), accounts.ClientFromRequest(r))
	if errors.Is(err, accounts.ErrInvalidToken) {
		g.renderMessage(w, http.StatusBadRequest, "Link not valid", err.Error()+".", config.ManagementPrefix+"/login", "Go to login")
		return
	}
	if err != nil {
		log.Printf("Email login by link failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	g.startSessionAndRedirect(w, r, user, "email", redirect)
}

// redirectToLogin shows the login page with an error; the login page only
// renders known ?error= codes, so free text goes in a cookie-less re-render.
func (g *Gateway) redirectToLogin(w http.ResponseWriter, r *http.Request, message string) {
	g.renderLoginPage(w, r, http.StatusBadRequest, "", message)
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
		Email:           user.Email,
		Name:            user.Name,
		EmailConfirmed:  user.EmailConfirmed,
		CanConfirmEmail: g.Accounts.Options().BaseURL != "" && g.GatewayConfig.Notification.Email.IsConfigured(),
		HasPassword:     user.Password != "",
		CSRF:            accounts.CSRFToken(acct.session.Token),
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

func (g *Gateway) handleSendEmailConfirmation(w http.ResponseWriter, r *http.Request, acct *accountRequest) {
	err := g.Accounts.SendEmailConfirmation(r.Context(), acct.user)
	if errors.Is(err, accounts.ErrTooSoon) {
		g.renderAccountPage(w, http.StatusTooManyRequests, acct, "", err.Error()+".")
		return
	}
	if err != nil {
		log.Printf("Sending email confirmation to %s failed: %v", acct.user.ID, err)
		g.renderAccountPage(w, http.StatusInternalServerError, acct, "", "We could not send the email. Try again later.")
		return
	}
	accountRedirect(w, r, "msg", "email_sent")
}
