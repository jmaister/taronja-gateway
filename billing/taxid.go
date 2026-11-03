package billing

import (
	"fmt"
	"strings"
)

// Customer kinds.
const (
	KindIndividual = "individual"
	KindCompany    = "company"
)

// NormalizeTaxID uppercases a tax id and drops separators; for Spain it also
// drops the ES prefix, as Stripe's es_cif type and the database keep it.
func NormalizeTaxID(taxID, country string) string {
	v := strings.ToUpper(strings.NewReplacer(" ", "", "-", "", ".", "", "/", "").Replace(taxID))
	if country == "ES" {
		v = strings.TrimPrefix(v, "ES")
	}
	return v
}

// ValidateTaxID checks a normalized tax id. Spanish ids are verified with their
// check letter or digit: a company needs a CIF, an individual a NIF or NIE. Other
// countries only get a sanity check on length and characters.
func ValidateTaxID(taxID, kind, country string) error {
	if country != "ES" {
		if len(taxID) < 4 || len(taxID) > 20 || strings.Trim(taxID, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") != "" {
			return fmt.Errorf("%w: tax id %q is not valid", ErrInvalid, taxID)
		}
		return nil
	}
	switch idKind := spanishIDKind(taxID); {
	case idKind == "":
		return fmt.Errorf("%w: %q is not a valid Spanish NIF, NIE or CIF", ErrInvalid, taxID)
	case kind == KindCompany && idKind != "cif":
		return fmt.Errorf("%w: a company needs a CIF; %q is a personal NIF/NIE", ErrInvalid, taxID)
	case kind == KindIndividual && idKind == "cif":
		return fmt.Errorf("%w: an individual needs a NIF or NIE; %q is a CIF", ErrInvalid, taxID)
	}
	return nil
}

const dniLetters = "TRWAGMYFPDXBNJZSQVHLCKE"

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// spanishIDKind returns "nif", "nie" or "cif" for a valid Spanish id (no
// prefix, uppercase), "" for anything else.
func spanishIDKind(id string) string {
	if len(id) != 9 {
		return ""
	}
	first, body, last := id[0], id[1:8], id[8]
	if !isDigits(body) {
		return ""
	}
	switch {
	case first >= '0' && first <= '9':
		n := 0
		for _, r := range id[:8] {
			if r < '0' || r > '9' {
				return ""
			}
			n = n*10 + int(r-'0')
		}
		if dniLetters[n%23] == last {
			return "nif"
		}
	case first == 'X' || first == 'Y' || first == 'Z':
		n := int(strings.IndexByte("XYZ", first))*10_000_000 + atoi(body)
		if dniLetters[n%23] == last {
			return "nie"
		}
	case strings.IndexByte("ABCDEFGHJNPQRSUVW", first) >= 0:
		if cifControlOK(first, body, last) {
			return "cif"
		}
	}
	return ""
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func cifControlOK(first byte, body string, last byte) bool {
	sum := 0
	for i, r := range body {
		d := int(r - '0')
		if i%2 == 0 {
			d *= 2
			d = d/10 + d%10
		}
		sum += d
	}
	digit := (10 - sum%10) % 10
	letter := "JABCDEFGHI"[digit]
	switch {
	case strings.IndexByte("PQRSNW", first) >= 0:
		return last == letter
	case strings.IndexByte("ABEH", first) >= 0:
		return last == byte('0'+digit)
	}
	return last == letter || last == byte('0'+digit)
}
