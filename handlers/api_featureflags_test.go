package handlers

import (
	"context"
	"testing"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/featureflag"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeatureFlagHandlers(t *testing.T) {
	db.SetupTestDB(t.Name())
	server := &StrictApiServer{}
	server.SetFeatureFlags(featureflag.NewService(repositories.NewFeatureFlagRepositoryDB(db.GetConnection())))

	adminCtx := context.WithValue(context.Background(), session.SessionKey, &db.Session{UserID: "admin", IsAuthenticated: true, IsAdmin: true})
	userCtx := context.WithValue(context.Background(), session.SessionKey, &db.Session{UserID: "user", IsAuthenticated: true})
	anonCtx := context.Background()

	t.Run("admin endpoints reject non-admins", func(t *testing.T) {
		resp, err := server.ListFeatureFlags(anonCtx, api.ListFeatureFlagsRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, api.ListFeatureFlags401JSONResponse{}, resp)

		created, err := server.CreateFeatureFlag(userCtx, api.CreateFeatureFlagRequestObject{Body: &api.FeatureFlagCreateRequest{Key: "x"}})
		require.NoError(t, err)
		assert.IsType(t, api.CreateFeatureFlag403JSONResponse{}, created)
	})

	t.Run("create, update, read values, delete", func(t *testing.T) {
		enabled := true
		desc := "Checkout v2"
		resp, err := server.CreateFeatureFlag(adminCtx, api.CreateFeatureFlagRequestObject{Body: &api.FeatureFlagCreateRequest{Key: "new-checkout", Description: &desc, Enabled: &enabled}})
		require.NoError(t, err)
		created, ok := resp.(api.CreateFeatureFlag201JSONResponse)
		require.True(t, ok, "got %T", resp)
		assert.Equal(t, api.FeatureFlagType("boolean"), created.Type)
		assert.True(t, created.Enabled)

		dup, err := server.CreateFeatureFlag(adminCtx, api.CreateFeatureFlagRequestObject{Body: &api.FeatureFlagCreateRequest{Key: "new-checkout"}})
		require.NoError(t, err)
		assert.IsType(t, api.CreateFeatureFlag409JSONResponse{}, dup)

		bad, err := server.CreateFeatureFlag(adminCtx, api.CreateFeatureFlagRequestObject{Body: &api.FeatureFlagCreateRequest{Key: "Not Valid"}})
		require.NoError(t, err)
		assert.IsType(t, api.CreateFeatureFlag400JSONResponse{}, bad)

		values, err := server.GetFeatureFlagValues(anonCtx, api.GetFeatureFlagValuesRequestObject{})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"new-checkout": true}, values.(api.GetFeatureFlagValues200JSONResponse).Flags)

		off := false
		updated, err := server.UpdateFeatureFlag(adminCtx, api.UpdateFeatureFlagRequestObject{Key: "new-checkout", Body: &api.FeatureFlagUpdateRequest{Enabled: &off}})
		require.NoError(t, err)
		assert.False(t, updated.(api.UpdateFeatureFlag200JSONResponse).Enabled)
		assert.Equal(t, "Checkout v2", updated.(api.UpdateFeatureFlag200JSONResponse).Description)

		missing, err := server.UpdateFeatureFlag(adminCtx, api.UpdateFeatureFlagRequestObject{Key: "nope", Body: &api.FeatureFlagUpdateRequest{Enabled: &off}})
		require.NoError(t, err)
		assert.IsType(t, api.UpdateFeatureFlag404JSONResponse{}, missing)

		list, err := server.ListFeatureFlags(adminCtx, api.ListFeatureFlagsRequestObject{})
		require.NoError(t, err)
		assert.Len(t, list.(api.ListFeatureFlags200JSONResponse), 1)

		deleted, err := server.DeleteFeatureFlag(adminCtx, api.DeleteFeatureFlagRequestObject{Key: "new-checkout"})
		require.NoError(t, err)
		assert.IsType(t, api.DeleteFeatureFlag204Response{}, deleted)

		gone, err := server.GetFeatureFlag(adminCtx, api.GetFeatureFlagRequestObject{Key: "new-checkout"})
		require.NoError(t, err)
		assert.IsType(t, api.GetFeatureFlag404JSONResponse{}, gone)
	})
}
