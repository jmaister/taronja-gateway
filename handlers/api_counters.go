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

// getUserCounters handles GET /api/counters/{counterId}/{userId}
func (s *StrictApiServer) GetUserCounters(ctx context.Context, request api.GetUserCountersRequestObject) (api.GetUserCountersResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.GetUserCounters401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	userID := request.UserId
	counterID := request.CounterId

	// Users can only view their own counters unless they are admin
	if sessionObj.UserID != userID && !sessionObj.IsAdmin {
		return api.GetUserCounters403JSONResponse{
			Code:    403,
			Message: "Forbidden: can only view own counters",
		}, nil
	}

	// Verify user exists
	user, err := s.userRepo.FindUserByIdOrUsername(userID, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return api.GetUserCounters404JSONResponse{
				Code:    404,
				Message: "User not found",
			}, nil
		}
		return api.GetUserCounters500JSONResponse{
			Code:    500,
			Message: "Internal server error",
		}, nil
	}

	// Get user's counter balance
	balance, err := s.countersRepo.GetUserBalance(userID, counterID)
	if err != nil {
		return api.GetUserCounters500JSONResponse{
			Code:    500,
			Message: "Failed to get user counters",
		}, nil
	}

	// Prepare email for response
	var emailPtr *types.Email
	if user.Email != "" {
		emailVal := types.Email(user.Email)
		emailPtr = &emailVal
	}

	return api.GetUserCounters200JSONResponse{
		UserId:      balance.UserID,
		Username:    &user.Username,
		Email:       emailPtr,
		CounterId:   balance.CounterID,
		Balance:     balance.Balance,
		LastUpdated: balance.LastUpdated,
		HasHistory:  balance.HasHistory,
	}, nil
}

// adjustUserCounters handles POST /api/counters/{counterId}/{userId}
func (s *StrictApiServer) AdjustUserCounters(ctx context.Context, request api.AdjustUserCountersRequestObject) (api.AdjustUserCountersResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.AdjustUserCounters401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	// Only admins can adjust counters
	if !sessionObj.IsAdmin {
		return api.AdjustUserCounters403JSONResponse{
			Code:    403,
			Message: "Forbidden: admin access required",
		}, nil
	}

	userID := request.UserId
	counterID := request.CounterId

	// Verify user exists
	_, err := s.userRepo.FindUserByIdOrUsername(userID, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return api.AdjustUserCounters404JSONResponse{
				Code:    404,
				Message: "User not found",
			}, nil
		}
		return api.AdjustUserCounters500JSONResponse{
			Code:    500,
			Message: "Internal server error",
		}, nil
	}

	// Validate request body
	if request.Body.Amount == 0 {
		return api.AdjustUserCounters400JSONResponse{
			Code:    400,
			Message: "Amount cannot be zero",
		}, nil
	}

	if request.Body.Description == "" {
		return api.AdjustUserCounters400JSONResponse{
			Code:    400,
			Message: "Description is required",
		}, nil
	}

	// Adjust counters
	transaction, err := s.countersRepo.AdjustCounters(userID, counterID, request.Body.Amount, request.Body.Description)
	if err != nil {
		if err.Error() == "insufficient counters: transaction would result in negative balance" {
			return api.AdjustUserCounters400JSONResponse{
				Code:    400,
				Message: "Insufficient counters",
			}, nil
		}
		return api.AdjustUserCounters500JSONResponse{
			Code:    500,
			Message: "Failed to adjust counters",
		}, nil
	}

	return api.AdjustUserCounters200JSONResponse{
		Id:           transaction.ID,
		UserId:       transaction.UserID,
		CounterId:    transaction.CounterID,
		Amount:       transaction.Amount,
		BalanceAfter: transaction.BalanceAfter,
		Description:  transaction.Description,
		CreatedAt:    transaction.CreatedAt,
	}, nil
}

// getUserCounterHistory handles GET /api/counters/{counterId}/{userId}/history
func (s *StrictApiServer) GetUserCounterHistory(ctx context.Context, request api.GetUserCounterHistoryRequestObject) (api.GetUserCounterHistoryResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.GetUserCounterHistory401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	userID := request.UserId
	counterID := request.CounterId

	// Users can only view their own history unless they are admin
	if sessionObj.UserID != userID && !sessionObj.IsAdmin {
		return api.GetUserCounterHistory403JSONResponse{
			Code:    403,
			Message: "Forbidden: can only view own counter history",
		}, nil
	}

	// Verify user exists
	_, err := s.userRepo.FindUserByIdOrUsername(userID, "", "")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return api.GetUserCounterHistory404JSONResponse{
				Code:    404,
				Message: "User not found",
			}, nil
		}
		return api.GetUserCounterHistory500JSONResponse{
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

	// Get counter history
	history, err := s.countersRepo.GetCounterHistory(userID, counterID, limit, offset)
	if err != nil {
		return api.GetUserCounterHistory500JSONResponse{
			Code:    500,
			Message: "Failed to get counter history",
		}, nil
	}

	// Convert transactions to API format
	transactions := make([]api.CounterTransactionResponse, len(history.Transactions))
	for i, tx := range history.Transactions {
		transactions[i] = api.CounterTransactionResponse{
			Id:           tx.ID,
			UserId:       tx.UserID,
			CounterId:    tx.CounterID,
			Amount:       tx.Amount,
			BalanceAfter: tx.BalanceAfter,
			Description:  tx.Description,
			CreatedAt:    tx.CreatedAt,
		}
	}

	return api.GetUserCounterHistory200JSONResponse{
		UserId:         history.UserID,
		CounterId:      history.CounterID,
		CurrentBalance: history.Balance,
		HasHistory:     history.HasHistory,
		Transactions:   transactions,
		TotalCount:     int(history.TotalCount),
		Limit:          history.Limit,
		Offset:         history.Offset,
	}, nil
}

// getAllUserCounters handles GET /api/admin/counters/{counterId}
func (s *StrictApiServer) GetAllUserCounters(ctx context.Context, request api.GetAllUserCountersRequestObject) (api.GetAllUserCountersResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.GetAllUserCounters401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	// Only admins can view all user counters
	if !sessionObj.IsAdmin {
		return api.GetAllUserCounters403JSONResponse{
			Code:    403,
			Message: "Forbidden: admin access required",
		}, nil
	}

	counterID := request.CounterId

	// Parse pagination parameters
	limit := 50 // default
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	offset := 0 // default
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	// Get all user counters
	result, err := s.countersRepo.GetAllUserCounters(counterID, limit, offset)
	if err != nil {
		return api.GetAllUserCounters500JSONResponse{
			Code:    500,
			Message: "Failed to get user counters",
		}, nil
	}

	// Convert to API format - need to match the generated struct exactly
	users := make([]api.UserCountersResponse, len(result.Users))

	for i, user := range result.Users {
		var emailPtr *types.Email
		if user.Email != "" {
			emailVal := types.Email(user.Email)
			emailPtr = &emailVal
		}
		users[i] = api.UserCountersResponse{
			UserId:      user.UserID,
			Username:    &user.Username,
			Email:       emailPtr,
			CounterId:   user.CounterID,
			Balance:     user.Balance,
			LastUpdated: user.LastUpdated,
			HasHistory:  user.HasHistory,
		}
	}

	return api.GetAllUserCounters200JSONResponse{
		CounterId:  result.CounterID,
		Users:      users,
		TotalCount: int(result.TotalCount),
		Limit:      result.Limit,
		Offset:     result.Offset,
	}, nil
}

// getAvailableCounters handles GET /api/admin/counters
func (s *StrictApiServer) GetAvailableCounters(ctx context.Context, request api.GetAvailableCountersRequestObject) (api.GetAvailableCountersResponseObject, error) {
	// Get session from context
	sessionObj, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || sessionObj == nil {
		return api.GetAvailableCounters401JSONResponse{
			Code:    401,
			Message: "Unauthorized",
		}, nil
	}

	// Only admins can get available counters
	if !sessionObj.IsAdmin {
		return api.GetAvailableCounters403JSONResponse{
			Code:    403,
			Message: "Forbidden: admin access required",
		}, nil
	}

	// Get available counter types from the repository
	counterTypes, err := s.countersRepo.GetAvailableCounterTypes()
	if err != nil {
		return api.GetAvailableCounters500JSONResponse{
			Code:    500,
			Message: "Internal server error",
		}, nil
	}

	return api.GetAvailableCounters200JSONResponse{
		Counters: counterTypes,
	}, nil
}
