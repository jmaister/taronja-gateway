package fingerprint

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectFingerprint(t *testing.T) {
	tests := []struct {
		name          string
		setHeaders    map[string]string
		expectedValue string
		expectedType  string
	}{
		{
			name:          "nothing set",
			setHeaders:    map[string]string{},
			expectedValue: "",
			expectedType:  "",
		},
		{
			name:          "only JA4H",
			setHeaders:    map[string]string{JA4HHeaderName: "ja4h-value"},
			expectedValue: "ja4h-value",
			expectedType:  TypeJA4H,
		},
		{
			name: "stable outranks JA4H",
			setHeaders: map[string]string{
				JA4HHeaderName:              "ja4h-value",
				StableFingerprintHeaderName: "stable-value",
			},
			expectedValue: "stable-value",
			expectedType:  TypeStable,
		},
		{
			name: "TLS JA4 outranks stable and JA4H",
			setHeaders: map[string]string{
				JA4HHeaderName:              "ja4h-value",
				StableFingerprintHeaderName: "stable-value",
				JA4TLSHeaderName:            "tls-value",
			},
			expectedValue: "tls-value",
			expectedType:  TypeJA4TLS,
		},
		{
			name: "only TLS JA4",
			setHeaders: map[string]string{
				JA4TLSHeaderName: "tls-value",
			},
			expectedValue: "tls-value",
			expectedType:  TypeJA4TLS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.setHeaders {
				req.Header.Set(k, v)
			}
			value, fpType := SelectFingerprint(req)
			assert.Equal(t, tt.expectedValue, value)
			assert.Equal(t, tt.expectedType, fpType)
		})
	}
}
