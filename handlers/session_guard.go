package handlers

import (
	"context"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// requireSession returns the session middleware.StrictSessionMiddleware put in
// ctx. A missing one means an internal inconsistency, so handlers answer 401
// rather than panic.
func requireSession(ctx context.Context) (*db.Session, bool) {
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return nil, false
	}
	return sessionObj, true
}

// requireAdminSession is requireSession plus an authenticated-admin check.
// Handlers build their own 401 from the boolean, as every OpenAPI operation
// has its own generated response type; the global middleware only requires
// "some valid session", so the admin boundary lives in each handler.
func requireAdminSession(ctx context.Context) (*db.Session, bool) {
	sessionObj, ok := requireSession(ctx)
	if !ok || !sessionObj.IsAuthenticated || !sessionObj.IsAdmin {
		return nil, false
	}
	return sessionObj, true
}
