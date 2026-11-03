package billing

import (
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
