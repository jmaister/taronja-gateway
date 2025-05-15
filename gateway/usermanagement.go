package gateway

import (
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/handlers"
)

// RegisterUserManagementRoutes registers all user management related routes
func (g *Gateway) RegisterUserManagementRoutes() {
	// Only register user management routes if authentication is configured
	if !g.GatewayConfig.HasAnyAuthentication() {
		log.Printf("Skipping User Management routes registration as no authentication methods are configured")
		return
	}

	// Register Create User Page Route
	g.registerCreateUserPageRoute()

	// Register Create User API Endpoint
	g.registerCreateUserAPIEndpoint()

	// Register Get User API Endpoint
	g.registerGetUserAPIEndpoint()

	// Register List Users Page Route
	g.registerListUsersPageRoute()

	log.Printf("Registered User Management routes")
}

// registerListUsersPageRoute adds the route for the list users page.
func (g *Gateway) registerListUsersPageRoute() {
	listUsersPath := g.GatewayConfig.Management.Prefix + "/admin/users"
	listUsersHandler := func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleListUsers(w, r, g.UserRepository, g.templates, g.GatewayConfig.Management.Prefix)
	}

	authWrappedListUsersHandler := g.wrapWithAuth(listUsersHandler, false) // false for not static
	g.Mux.HandleFunc("GET "+listUsersPath, authWrappedListUsersHandler)
	log.Printf("Registered Management Route: %-25s | Path: %s | Method: GET | Auth: %t", "List Users Page", listUsersPath, true)
}

// registerCreateUserPageRoute adds the route for the create user page.
func (g *Gateway) registerCreateUserPageRoute() {
	createUserPath := g.GatewayConfig.Management.Prefix + "/admin/users_new"
	// Create a handler that passes the session store to HandleMe (similar pattern for auth)
	createUserHandler := func(w http.ResponseWriter, r *http.Request) {
		// Retrieve the pre-parsed template from the map
		tmpl, ok := g.templates["create_user.html"]
		if !ok || tmpl == nil {
			log.Printf("Error: Create user template 'create_user.html' not found in cache")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Add management prefix to template data, use a map for dynamic data
		data := map[string]interface{}{
			"ManagementPrefix": g.GatewayConfig.Management.Prefix,
		}

		// Execute the template
		err := tmpl.Execute(w, data)
		if err != nil {
			log.Printf("Error executing create_user template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	authWrappedCreateUserHandler := g.wrapWithAuth(createUserHandler, false) // false for not static
	g.Mux.HandleFunc(createUserPath, authWrappedCreateUserHandler)
	log.Printf("Registered Management Route: %-25s | Path: %s | Auth: %t", "Create User Page", createUserPath, true)
}

// registerCreateUserAPIEndpoint adds the API endpoint for creating a new user.
func (g *Gateway) registerCreateUserAPIEndpoint() {
	createUserAPIPath := g.GatewayConfig.Management.Prefix + "/api/user" // As per user request

	// The actual handler logic will be in handlers.HandleCreateUser
	// We need to ensure this handler is also wrapped with authentication
	createUserAPIHandler := func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleCreateUser(w, r, g.UserRepository)
	}

	authWrappedCreateUserAPIHandler := g.wrapWithAuth(createUserAPIHandler, false) // false for not static
	g.Mux.HandleFunc(createUserAPIPath, authWrappedCreateUserAPIHandler)
	log.Printf("Registered API Route: %-25s | Path: %s | Method: POST | Auth: %t", "Create User", createUserAPIPath, true)
}

// registerGetUserAPIEndpoint adds the API endpoint for retrieving a user by ID.
func (g *Gateway) registerGetUserAPIEndpoint() {
	// The path will be /_/admin/users/{user_id} to use Go 1.22+ path parameters
	getUserAPIPath := g.GatewayConfig.Management.Prefix + "/admin/users/{user_id}"

	getUserAPIHandler := func(w http.ResponseWriter, r *http.Request) {
		// Pass g.templates, g.GatewayConfig.Management.Prefix and g.SessionStore to HandleGetUser
		handlers.HandleGetUser(w, r, g.UserRepository, g.templates, g.GatewayConfig.Management.Prefix, g.SessionRepository)
	}

	authWrappedGetUserAPIHandler := g.wrapWithAuth(getUserAPIHandler, false) // false for not static
	// Register with HTTP method for pattern matching
	g.Mux.HandleFunc("GET "+getUserAPIPath, authWrappedGetUserAPIHandler)
	log.Printf("Registered API Route: %-25s | Path: %s | Method: GET | Auth: %t", "Get User", getUserAPIPath, true)
}
