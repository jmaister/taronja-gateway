package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// ListTokens handles GET /api/tokens
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

	// Get user tokens
	tokens, err := s.tokenService.GetUserTokens(sessionObj.UserID)
	if err != nil {
		log.Printf("ListTokens: Error getting tokens for user %s: %v", sessionObj.UserID, err)
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

// CreateToken handles POST /api/tokens
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

	// Get client info from session (if available)
	var clientInfo *db.ClientInfo
	if sessionObj != nil {
		clientInfo = &sessionObj.ClientInfo
	}

	// Generate token
	tokenString, tokenData, err := s.tokenService.GenerateToken(
		sessionObj.UserID,
		request.Body.Name,
		request.Body.ExpiresAt,
		scopes,
		"api_request",
		clientInfo,
	)
	if err != nil {
		log.Printf("CreateToken: Error generating token for user %s: %v", sessionObj.UserID, err)
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

	// Get user tokens and find the requested one
	tokens, err := s.tokenService.GetUserTokens(sessionObj.UserID)
	if err != nil {
		log.Printf("GetToken: Error getting tokens for user %s: %v", sessionObj.UserID, err)
		return api.GetToken500JSONResponse{
			Code:    500,
			Message: "Internal server error: Failed to retrieve tokens",
		}, nil
	}

	// Find the specific token
	var targetToken *db.Token
	for _, token := range tokens {
		if token.ID == request.TokenId {
			targetToken = token
			break
		}
	}

	if targetToken == nil {
		return api.GetToken404JSONResponse{
			Code:    404,
			Message: "Not found: Token not found or does not belong to user",
		}, nil
	}

	return api.GetToken200JSONResponse(convertTokenToResponse(targetToken)), nil
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

	// Revoke the token
	err := s.tokenService.RevokeToken(request.TokenId, sessionObj.UserID, sessionObj.UserID)
	if err != nil {
		log.Printf("DeleteToken: Error revoking token %s for user %s: %v", request.TokenId, sessionObj.UserID, err)
		
		// Check if it's a not found / not belonging to user error
		errMsg := err.Error()
		if errMsg == "token does not belong to user" || 
		   errMsg == "token not found" || 
		   errMsg == "token not found: record not found" ||
		   strings.Contains(errMsg, "token with ID") && strings.Contains(errMsg, "not found") {
			return api.DeleteToken404JSONResponse{
				Code:    404,
				Message: "Not found: Token not found or does not belong to user",
			}, nil
		}

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
