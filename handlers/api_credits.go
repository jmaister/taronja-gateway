package handlers

import (
	"context"
	"errors"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"
)

// getUserCredits handles GET /api/credits/{userId}
func (s *StrictApiServer) GetUserCredits(ctx context.Context, request api.GetUserCreditsRequestObject) (api.GetUserCreditsResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.GetUserCredits401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	userID := request.UserId

	// Users can only view their own credits unless they are admin
	if sessionObj.UserID != userID && !sessionObj.IsAdmin {
		return api.GetUserCredits403JSONResponse{
			Code:    403,
			Message: "Forbidden: can only view own credits",
		}, nil
	}

	// Verify user exists
	_, err := s.userRepo.FindUserByIdOrUsername(userID, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return api.GetUserCredits404JSONResponse{
				Code:    404,
				Message: "User not found",
			}, nil
		}
		return api.GetUserCredits500JSONResponse{
			Code:    500,
			Message: "Internal server error",
		}, nil
	}

	// Get user's credit balance
	balance, err := s.creditsRepo.GetUserBalance(userID)
	if err != nil {
		return api.GetUserCredits500JSONResponse{
			Code:    500,
			Message: "Failed to get user credits",
		}, nil
	}

	return api.GetUserCredits200JSONResponse{
		UserId:      balance.UserID,
		Balance:     balance.Balance,
		LastUpdated: balance.LastUpdated,
	}, nil
}

// adjustUserCredits handles POST /api/credits/{userId}
func (s *StrictApiServer) AdjustUserCredits(ctx context.Context, request api.AdjustUserCreditsRequestObject) (api.AdjustUserCreditsResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.AdjustUserCredits401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	// Only admins can adjust credits
	if !sessionObj.IsAdmin {
		return api.AdjustUserCredits403JSONResponse{
			Code:    403,
			Message: "Forbidden: admin access required",
		}, nil
	}

	userID := request.UserId

	// Verify user exists
	_, err := s.userRepo.FindUserByIdOrUsername(userID, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return api.AdjustUserCredits404JSONResponse{
				Code:    404,
				Message: "User not found",
			}, nil
		}
		return api.AdjustUserCredits500JSONResponse{
			Code:    500,
			Message: "Internal server error",
		}, nil
	}

	// Validate request body
	if request.Body.Amount == 0 {
		return api.AdjustUserCredits400JSONResponse{
			Code:    400,
			Message: "Amount cannot be zero",
		}, nil
	}

	if request.Body.Description == "" {
		return api.AdjustUserCredits400JSONResponse{
			Code:    400,
			Message: "Description is required",
		}, nil
	}

	// Adjust credits
	transaction, err := s.creditsRepo.AdjustCredits(userID, request.Body.Amount, request.Body.Description)
	if err != nil {
		if err.Error() == "insufficient credits: transaction would result in negative balance" {
			return api.AdjustUserCredits400JSONResponse{
				Code:    400,
				Message: "Insufficient credits",
			}, nil
		}
		return api.AdjustUserCredits500JSONResponse{
			Code:    500,
			Message: "Failed to adjust credits",
		}, nil
	}

	return api.AdjustUserCredits200JSONResponse{
		Id:           transaction.ID,
		UserId:       transaction.UserID,
		Amount:       transaction.Amount,
		BalanceAfter: transaction.BalanceAfter,
		Description:  transaction.Description,
		CreatedAt:    transaction.CreatedAt,
	}, nil
}

// getUserCreditHistory handles GET /api/credits/{userId}/history
func (s *StrictApiServer) GetUserCreditHistory(ctx context.Context, request api.GetUserCreditHistoryRequestObject) (api.GetUserCreditHistoryResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.GetUserCreditHistory401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	userID := request.UserId

	// Users can only view their own history unless they are admin
	if sessionObj.UserID != userID && !sessionObj.IsAdmin {
		return api.GetUserCreditHistory403JSONResponse{
			Code:    403,
			Message: "Forbidden: can only view own credit history",
		}, nil
	}

	// Verify user exists
	_, err := s.userRepo.FindUserByIdOrUsername(userID, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return api.GetUserCreditHistory404JSONResponse{
				Code:    404,
				Message: "User not found",
			}, nil
		}
		return api.GetUserCreditHistory500JSONResponse{
			Code:    500,
			Message: "Internal server error",
		}, nil
	}

	// Parse pagination parameters
	limit := 50 // default
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	offset := 0 // default
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	// Get credit history
	history, err := s.creditsRepo.GetCreditHistory(userID, limit, offset)
	if err != nil {
		return api.GetUserCreditHistory500JSONResponse{
			Code:    500,
			Message: "Failed to get credit history",
		}, nil
	}

	// Convert transactions to API format
	transactions := make([]api.CreditTransactionResponse, len(history.Transactions))
	for i, tx := range history.Transactions {
		transactions[i] = api.CreditTransactionResponse{
			Id:           tx.ID,
			UserId:       tx.UserID,
			Amount:       tx.Amount,
			BalanceAfter: tx.BalanceAfter,
			Description:  tx.Description,
			CreatedAt:    tx.CreatedAt,
		}
	}

	return api.GetUserCreditHistory200JSONResponse{
		UserId:         history.UserID,
		CurrentBalance: history.Balance,
		Transactions:   transactions,
		TotalCount:     int(history.TotalCount),
		Limit:          history.Limit,
		Offset:         history.Offset,
	}, nil
}

// getAllUserCredits handles GET /api/admin/credits
func (s *StrictApiServer) GetAllUserCredits(ctx context.Context, request api.GetAllUserCreditsRequestObject) (api.GetAllUserCreditsResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.GetAllUserCredits401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	// Only admins can view all user credits
	if !sessionObj.IsAdmin {
		return api.GetAllUserCredits403JSONResponse{
			Code:    403,
			Message: "Forbidden: admin access required",
		}, nil
	}

	// Parse pagination parameters
	limit := 50 // default
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	offset := 0 // default
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	// Get all user credits
	result, err := s.creditsRepo.GetAllUserCredits(limit, offset)
	if err != nil {
		return api.GetAllUserCredits500JSONResponse{
			Code:    500,
			Message: "Failed to get user credits",
		}, nil
	}

	// Convert to API format - need to match the generated struct exactly
	users := make([]api.UserCreditsResponse, len(result.Users))

	for i, user := range result.Users {
		var emailPtr *types.Email
		if user.Email != "" {
			emailVal := types.Email(user.Email)
			emailPtr = &emailVal
		}
		users[i] = api.UserCreditsResponse{
			Balance:     user.Balance,
			LastUpdated: user.LastUpdated,
			UserId:      user.UserID,
			Username:    &user.Username,
			Email:       emailPtr,
		}
	}

	return api.GetAllUserCredits200JSONResponse{
		Users:      users,
		TotalCount: int(result.TotalCount),
		Limit:      result.Limit,
		Offset:     result.Offset,
	}, nil
}
