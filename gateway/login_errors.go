package gateway

import "github.com/jmaister/taronja-gateway/accounts"

// loginErrors are the messages the login page shows for ?error= codes set by
// the external login callbacks.
var loginErrors = map[string]string{
	"link_required":      accounts.ErrLinkRequired.Error(),
	"email_not_verified": accounts.ErrEmailNotVerified.Error() + ". Verify it with the provider, or use another way to log in.",
}
