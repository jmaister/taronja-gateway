package fingerprint

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetJA4FromRequest(t *testing.T) {
	tests := []struct {
		name           string
		headerValue    string
		expectedResult string
	}{
		{
			name:           "returns_ja4h_from_header",
			headerValue:    "test_fingerprint_123",
			expectedResult: "test_fingerprint_123",
		},
		{
			name:           "returns_empty_when_header_missing",
			headerValue:    "",
			expectedResult: "",
		},
		{
			name:           "returns_complex_ja4h_value",
			headerValue:    "ja4h_value_with-special.chars_12345",
			expectedResult: "ja4h_value_with-special.chars_12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: make(http.Header),
			}
			if tt.headerValue != "" {
				req.Header.Set(JA4HHeaderName, tt.headerValue)
			}

			result := GetJA4FromRequest(req)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestJA4HConstants(t *testing.T) {
	assert.Equal(t, "X-Taronja-JA4H", JA4HHeaderName)
	assert.Equal(t, JA4HKeyType("ja4h_fingerprint"), JA4HKey)
}
