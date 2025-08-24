package handlers

import (
	"context"
	"log"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// ListTokens handles GET /api/users/{userId}/tokens
func (s *StrictApiServer) ListTokens(ctx context.Context, request api.ListTokensRequestObject) (api.ListTokensResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		log.Printf("ListTokens: No session found in context")
		return api.ListTokens401JSONResponse{
			Code:    401,
			Message: "Unauthorized: No valid session found",
		}, nil
	}

	// Check if user is admin
	if !sessionObj.IsAdmin {
		log.Printf("ListTokens: Non-admin user %s attempted to access tokens", sessionObj.UserID)
		return api.ListTokens401JSONResponse{
			Code:    401,
			Message: "Unauthorized: Admin access required",
		}, nil
	}

	// Validate that the user exists
	user, err := s.userRepo.FindUserByIdOrUsername(request.UserId, "", "")
	if err != nil || user == nil {
		log.Printf("ListTokens: User not found with ID %s: %v", request.UserId, err)
		return api.ListTokens404JSONResponse{
			Code:    404,
			Message: "User not found",
		}, nil
	}

	// Get user tokens
	tokens, err := s.tokenService.GetUserTokens(user.ID)
	if err != nil {
		log.Printf("ListTokens: Error getting tokens for user %s: %v", user.ID, err)
		return api.ListTokens500JSONResponse{
			Code:    500,
			Message: "Internal server error: Failed to retrieve tokens",
		}, nil
	}

	// Convert to API response format
	var tokenResponses []api.TokenResponse
	for _, token := range tokens {
		tokenResponses = append(tokenResponses, convertTokenToResponse(token))
	}

	return api.ListTokens200JSONResponse(tokenResponses), nil
}

// CreateToken handles POST /api/users/{userId}/tokens
func (s *StrictApiServer) CreateToken(ctx context.Context, request api.CreateTokenRequestObject) (api.CreateTokenResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		log.Printf("CreateToken: No session found in context")
		return api.CreateToken401JSONResponse{
			Code:    401,
			Message: "Unauthorized: No valid session found",
		}, nil
	}

	// Check if user is admin
	if !sessionObj.IsAdmin {
		log.Printf("CreateToken: Non-admin user %s attempted to create token", sessionObj.UserID)
		return api.CreateToken401JSONResponse{
			Code:    401,
			Message: "Unauthorized: Admin access required",
		}, nil
	}

	// Validate that the user exists
	user, err := s.userRepo.FindUserByIdOrUsername(request.UserId, "", "")
	if err != nil || user == nil {
		log.Printf("CreateToken: User not found with ID %s: %v", request.UserId, err)
		return api.CreateToken404JSONResponse{
			Code:    404,
			Message: "User not found",
		}, nil
	}

	// Validate request
	if request.Body.Name == "" {
		return api.CreateToken400JSONResponse{
			Code:    400,
			Message: "Bad request: Token name is required",
		}, nil
	}

	// Check if expiration is in the future
	if request.Body.ExpiresAt != nil && request.Body.ExpiresAt.Before(time.Now()) {
		return api.CreateToken400JSONResponse{
			Code:    400,
			Message: "Bad request: Expiration date must be in the future",
		}, nil
	}

	// Convert scopes to string slice
	var scopes []string
	if request.Body.Scopes != nil {
		scopes = *request.Body.Scopes
	}

	// Get client info from session
	clientInfo := &sessionObj.ClientInfo

	// Generate token for the specified user
	tokenString, tokenData, err := s.tokenService.GenerateToken(
		user.ID,
		request.Body.Name,
		request.Body.ExpiresAt,
		scopes,
		"admin_api_request",
		clientInfo,
	)
	if err != nil {
		log.Printf("CreateToken: Error generating token for user %s: %v", user.ID, err)
		return api.CreateToken500JSONResponse{
			Code:    500,
			Message: "Internal server error: Failed to create token",
		}, nil
	}

	// Return the token and token info
	response := api.TokenCreateResponse{
		Token:     tokenString,
		TokenInfo: convertTokenToResponse(tokenData),
	}

	return api.CreateToken201JSONResponse(response), nil
}

// GetToken handles GET /api/tokens/{tokenId}
func (s *StrictApiServer) GetToken(ctx context.Context, request api.GetTokenRequestObject) (api.GetTokenResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		log.Printf("GetToken: No session found in context")
		return api.GetToken401JSONResponse{
			Code:    401,
			Message: "Unauthorized: No valid session found",
		}, nil
	}

	// Check if user is admin
	if !sessionObj.IsAdmin {
		log.Printf("GetToken: Non-admin user %s attempted to access token", sessionObj.UserID)
		return api.GetToken401JSONResponse{
			Code:    401,
			Message: "Unauthorized: Admin access required",
		}, nil
	}

	// Get the token directly by ID from the repository
	token, err := s.tokenRepo.GetTokenByID(request.TokenId)
	if err != nil {
		log.Printf("GetToken: Token not found with ID %s: %v", request.TokenId, err)
		return api.GetToken404JSONResponse{
			Code:    404,
			Message: "Token not found",
		}, nil
	}

	return api.GetToken200JSONResponse(convertTokenToResponse(token)), nil
}

// DeleteToken handles DELETE /api/tokens/{tokenId}
func (s *StrictApiServer) DeleteToken(ctx context.Context, request api.DeleteTokenRequestObject) (api.DeleteTokenResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		log.Printf("DeleteToken: No session found in context")
		return api.DeleteToken401JSONResponse{
			Code:    401,
			Message: "Unauthorized: No valid session found",
		}, nil
	}

	// Check if user is admin
	if !sessionObj.IsAdmin {
		log.Printf("DeleteToken: Non-admin user %s attempted to delete token", sessionObj.UserID)
		return api.DeleteToken401JSONResponse{
			Code:    401,
			Message: "Unauthorized: Admin access required",
		}, nil
	}

	// Get the token to check if it exists
	_, err := s.tokenRepo.GetTokenByID(request.TokenId)
	if err != nil {
		log.Printf("DeleteToken: Token not found with ID %s: %v", request.TokenId, err)
		return api.DeleteToken404JSONResponse{
			Code:    404,
			Message: "Token not found",
		}, nil
	}

	// Revoke the token (admin can revoke any token)
	err = s.tokenRepo.RevokeToken(request.TokenId, sessionObj.UserID)
	if err != nil {
		log.Printf("DeleteToken: Error revoking token %s by admin %s: %v", request.TokenId, sessionObj.UserID, err)
		return api.DeleteToken500JSONResponse{
			Code:    500,
			Message: "Internal server error: Failed to revoke token",
		}, nil
	}

	// Return success
	return api.DeleteToken204Response{}, nil
}

// Helper function to convert db.Token to api.TokenResponse
func convertTokenToResponse(token *db.Token) api.TokenResponse {
	response := api.TokenResponse{
		Id:         token.ID,
		Name:       token.Name,
		IsActive:   token.IsActive,
		CreatedAt:  token.CreatedAt,
		UsageCount: int(token.UsageCount),
		Scopes:     parseScopes(token.Scopes),
	}

	if token.ExpiresAt != nil {
		response.ExpiresAt = token.ExpiresAt
	}

	if token.LastUsedAt != nil {
		response.LastUsedAt = token.LastUsedAt
	}

	if token.RevokedAt != nil {
		response.RevokedAt = token.RevokedAt
	}

	return response
}

// Helper function to parse scopes from string format
func parseScopes(scopesStr string) []string {
	if scopesStr == "" {
		return []string{}
	}

	// Simple parsing - in a real implementation you might want to use JSON
	// For now, expect format like "[scope1,scope2,scope3]"
	if len(scopesStr) >= 2 && scopesStr[0] == '[' && scopesStr[len(scopesStr)-1] == ']' {
		content := scopesStr[1 : len(scopesStr)-1]
		if content == "" {
			return []string{}
		}

		// Split by comma and trim spaces
		scopes := []string{}
		for _, scope := range splitString(content, ",") {
			trimmed := trimString(scope)
			if trimmed != "" {
				scopes = append(scopes, trimmed)
			}
		}
		return scopes
	}

	return []string{}
}

// Helper function to split strings (simple implementation)
func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}

	var result []string
	start := 0

	for i := 0; i <= len(s)-len(sep); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i = start - 1
		}
	}

	result = append(result, s[start:])
	return result
}

// Helper function to trim whitespace (simple implementation)
func trimString(s string) string {
	start := 0
	end := len(s)

	// Trim leading whitespace
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	// Trim trailing whitespace
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}
