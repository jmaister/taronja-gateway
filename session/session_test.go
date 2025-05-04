package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateKey(t *testing.T) {
	store := NewMemorySessionStore()

	key, err := store.GenerateKey()
	assert.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.Len(t, key, 44) // base64 encoded 32 bytes
}

func TestSetAndGet(t *testing.T) {
	store := NewMemorySessionStore()
	key, _ := store.GenerateKey()

	sessionObj := SessionObject{
		Username:        "testuser",
		Email:           "test@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(1 * time.Hour),
		Provider:        "local",
	}

	err := store.Set(key, sessionObj)
	assert.NoError(t, err)

	retrieved, err := store.Get(key)
	assert.NoError(t, err)
	assert.Equal(t, sessionObj, retrieved)

	_, err = store.Get("nonexistent")
	assert.Error(t, err)
}

func TestValidate(t *testing.T) {
	store := NewMemorySessionStore()
	key, _ := store.GenerateKey()

	sessionObj := SessionObject{
		Username:        "testuser",
		Email:           "test@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(1 * time.Hour),
		Provider:        "local",
	}

	_ = store.Set(key, sessionObj)

	// Test with valid cookie
	req := httptest.NewRequest("GET", "/", nil)
	cookie := &http.Cookie{
		Name:  SessionCookieName,
		Value: key,
	}
	req.AddCookie(cookie)

	retrieved, valid := store.Validate(req)
	assert.True(t, valid)
	assert.Equal(t, sessionObj, retrieved)

	// Test with invalid cookie
	req = httptest.NewRequest("GET", "/", nil)
	cookie = &http.Cookie{
		Name:  SessionCookieName,
		Value: "invalid",
	}
	req.AddCookie(cookie)

	_, valid = store.Validate(req)
	assert.False(t, valid)

	// Test with no cookie
	req = httptest.NewRequest("GET", "/", nil)
	_, valid = store.Validate(req)
	assert.False(t, valid)
}
