package providers

import (
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/session"
)

func RegisterBasicAuthenticationCallback(g *Gateway, managementPrefix string) {
	// Basic Auth Login Route
	basicLoginPath := managementPrefix + "/auth/basic/login"
	g.mux.HandleFunc(basicLoginPath, func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate username and password (replace with your own logic)
		// TODO: load user from database
		if username == "admin" && password == "password" {
			// Generate session token
			token, err := g.sessionStore.GenerateKey()
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Store session
			so := session.SessionObject{
				Username: username,
				// TODO: add all fields
			}
			g.sessionStore.Set(token, so)

			// Set session token in a cookie
			http.SetCookie(w, &http.Cookie{
				Name:     session.SessionCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
			})

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Login successful"))
		} else {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		}
	})
	log.Printf("main.go: Registered Login Route: %-25s | Path: %s", "Basic Auth Login", basicLoginPath)

}
