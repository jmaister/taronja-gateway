package gateway

import (
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/providers"
)

// accountView is the account page: who is signed in, and their billing.
type accountView struct {
	Email          string
	Name           string
	EmailConfirmed bool
	CSRF           string
	Billing        *billingView
}

// registerAccountRoutes registers the account page, which holds the billing
// section.
func (g *Gateway) registerAccountRoutes() {
	if !g.GatewayConfig.HasAnyAuthentication() {
		return
	}
	g.Mux.HandleFunc("GET "+providers.AccountPath, g.handleAccountPage)
	log.Printf("Registered account routes")
}

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

func (g *Gateway) handleAccountPage(w http.ResponseWriter, r *http.Request) {
	acct, ok := g.currentAccount(r)
	if !ok {
		http.Redirect(w, r, config.ManagementPrefix+"/login?redirect="+providers.AccountPath, http.StatusFound)
		return
	}
	g.renderAccountPage(w, http.StatusOK, acct, "", "")
}

func (g *Gateway) renderAccountPage(w http.ResponseWriter, status int, acct *accountRequest, message, errorText string) {
	user := acct.user
	view := &accountView{
		Email:          user.Email,
		Name:           user.Name,
		EmailConfirmed: user.EmailConfirmed,
		CSRF:           accounts.CSRFToken(acct.session.Token),
		Billing:        g.accountBilling(acct),
	}
	page := g.newAuthPage("Your account")
	page.Wide = true
	page.Message = message
	page.Error = errorText
	page.Account = view
	g.renderAuthPage(w, status, "account.html", page)
}

func labelOr(labels map[string]string, key string) string {
	if label, ok := labels[key]; ok {
		return label
	}
	return key
}
