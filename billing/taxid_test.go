package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTaxID(t *testing.T) {
	cases := []struct {
		name, id, kind, country string
		ok                      bool
	}{
		{"NIF", "12345678Z", KindIndividual, "ES", true},
		{"NIF wrong letter", "12345678A", KindIndividual, "ES", false},
		{"NIE", "X1234567L", KindIndividual, "ES", true},
		{"NIE wrong letter", "X1234567A", KindIndividual, "ES", false},
		{"CIF digit control", "B12345674", KindCompany, "ES", true},
		{"CIF letter control", "A58818501", KindCompany, "ES", true},
		{"CIF wrong control", "B12345675", KindCompany, "ES", false},
		{"company with a NIF", "12345678Z", KindCompany, "ES", false},
		{"individual with a CIF", "B12345674", KindIndividual, "ES", false},
		{"special NIF K", "K1234567L", KindIndividual, "ES", true},
		{"special NIF L wrong letter", "L1234567A", KindIndividual, "ES", false},
		{"special NIF M", "M1234567L", KindIndividual, "ES", true},
		{"company with a special NIF", "K1234567L", KindCompany, "ES", false},
		{"garbage", "hola", KindCompany, "ES", false},
		{"other country", "FR12345678901", KindCompany, "FR", true},
		{"other country too short", "A1", KindCompany, "FR", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateTaxID(NormalizeTaxID(c.id, c.country), c.kind, c.country)
			if c.ok {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalid)
			}
		})
	}
}

func TestNormalizeTaxID(t *testing.T) {
	assert.Equal(t, "B12345674", NormalizeTaxID(" b-123 456.74 ", "ES"))
	assert.Equal(t, "B12345674", NormalizeTaxID("ESB12345674", "ES"))
	assert.Equal(t, "", NormalizeTaxID("  ", "ES"))
}

func TestVIES(t *testing.T) {
	var path string
	body := `{"isValid":true}`
	status := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	v := VIES{BaseURL: srv.URL}
	ctx := context.Background()

	ok, err := v.Check(ctx, "FR", "FR12345678901")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "/ms/FR/vat/12345678901", path)

	ok, err = v.Check(ctx, "GR", "EL123456789")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "/ms/EL/vat/123456789", path)

	body = `{"isValid":false}`
	ok, err = v.Check(ctx, "FR", "FR12345678901")
	assert.NoError(t, err)
	assert.False(t, ok)

	status, body = 500, `oops`
	_, err = v.Check(ctx, "FR", "FR12345678901")
	assert.Error(t, err)

	ok, err = v.Check(ctx, "US", "123456789")
	assert.NoError(t, err)
	assert.True(t, ok)
}
