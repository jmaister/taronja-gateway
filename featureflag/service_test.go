package featureflag_test

import (
	"testing"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/featureflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newService(t *testing.T) *featureflag.Service {
	t.Helper()
	db.SetupTestDB(t.Name())
	return featureflag.NewService(repositories.NewFeatureFlagRepositoryDB(db.GetConnection()))
}

func ptr[T any](v T) *T { return &v }

func TestService_CreateGetList(t *testing.T) {
	s := newService(t)

	created, err := s.Create(featureflag.CreateInput{Key: "new-checkout", Description: "Checkout v2", Enabled: true})
	require.NoError(t, err)
	assert.Equal(t, db.FeatureFlagTypeBoolean, created.Type)
	assert.True(t, created.Enabled)

	_, err = s.Create(featureflag.CreateInput{Key: "beta.banner"})
	require.NoError(t, err)

	got, err := s.Get("new-checkout")
	require.NoError(t, err)
	assert.Equal(t, "Checkout v2", got.Description)

	flags, err := s.List()
	require.NoError(t, err)
	require.Len(t, flags, 2)
	assert.Equal(t, "beta.banner", flags[0].Key, "ordered by key")
	assert.False(t, flags[0].Enabled)
}

func TestService_CreateRejectsBadInput(t *testing.T) {
	s := newService(t)

	for _, key := range []string{"", "Upper", "has space", "comma,key", "-leading", string(make([]byte, 101))} {
		_, err := s.Create(featureflag.CreateInput{Key: key})
		assert.ErrorIs(t, err, featureflag.ErrInvalidKey, "key %q", key)
	}

	_, err := s.Create(featureflag.CreateInput{Key: "typed", Type: "integer"})
	assert.ErrorIs(t, err, featureflag.ErrInvalidType)

	_, err = s.Create(featureflag.CreateInput{Key: "dup"})
	require.NoError(t, err)
	_, err = s.Create(featureflag.CreateInput{Key: "dup"})
	assert.ErrorIs(t, err, featureflag.ErrAlreadyExists)
}

func TestService_UpdateAndDelete(t *testing.T) {
	s := newService(t)
	_, err := s.Create(featureflag.CreateInput{Key: "dark-mode", Description: "old", Enabled: true})
	require.NoError(t, err)

	updated, err := s.Update("dark-mode", featureflag.UpdateInput{Enabled: ptr(false)})
	require.NoError(t, err)
	assert.False(t, updated.Enabled, "turning a flag off must persist the false value")
	assert.Equal(t, "old", updated.Description, "nil fields are left alone")

	updated, err = s.Update("dark-mode", featureflag.UpdateInput{Description: ptr("new")})
	require.NoError(t, err)
	assert.Equal(t, "new", updated.Description)
	assert.False(t, updated.Enabled)

	_, err = s.Update("missing", featureflag.UpdateInput{Enabled: ptr(true)})
	assert.ErrorIs(t, err, featureflag.ErrNotFound)

	require.NoError(t, s.Delete("dark-mode"))
	assert.ErrorIs(t, s.Delete("dark-mode"), featureflag.ErrNotFound)
	_, err = s.Get("dark-mode")
	assert.ErrorIs(t, err, featureflag.ErrNotFound)
}

func TestService_ValuesAndHeaderFollowWrites(t *testing.T) {
	s := newService(t)

	header, err := s.HeaderValue()
	require.NoError(t, err)
	assert.Equal(t, "", header)

	_, err = s.Create(featureflag.CreateInput{Key: "zeta", Enabled: true})
	require.NoError(t, err)
	_, err = s.Create(featureflag.CreateInput{Key: "alpha", Enabled: true})
	require.NoError(t, err)
	_, err = s.Create(featureflag.CreateInput{Key: "off-flag"})
	require.NoError(t, err)

	values, err := s.Values()
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"zeta": true, "alpha": true, "off-flag": false}, values)

	header, err = s.HeaderValue()
	require.NoError(t, err)
	assert.Equal(t, "alpha,zeta", header)

	_, err = s.Update("zeta", featureflag.UpdateInput{Enabled: ptr(false)})
	require.NoError(t, err)
	header, err = s.HeaderValue()
	require.NoError(t, err)
	assert.Equal(t, "alpha", header, "the cache is dropped on update")

	require.NoError(t, s.Delete("alpha"))
	values, err = s.Values()
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"zeta": false, "off-flag": false}, values, "the cache is dropped on delete")

	values["injected"] = true
	again, err := s.Values()
	require.NoError(t, err)
	assert.NotContains(t, again, "injected", "Values returns a copy")
}
