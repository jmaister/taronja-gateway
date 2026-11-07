package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
)

// CustomerDetails are the invoice details a customer gives before paying.
type CustomerDetails struct {
	Kind    string // KindIndividual or KindCompany
	Name    string
	TaxID   string // required for a company; optional for an individual
	Address Address
	Phone   string
}

// GetCustomerDetails returns what is saved for subject, for a user allowed to
// manage it; an empty db.BillingCustomer when nothing is saved yet.
func (s *Service) GetCustomerDetails(subject, userID string) (*db.BillingCustomer, error) {
	err := s.authorizeSubject(subject, userID)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.GetCustomer(subject)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &db.BillingCustomer{Subject: subject}, nil
	}
	return c, err
}

// SaveCustomerDetails validates and saves the invoice details of subject, and
// sends them to its Stripe customer (so Checkout shows them) and Odoo partner.
// A saved tax id never changes: another one is ErrConflict.
func (s *Service) SaveCustomerDetails(ctx context.Context, subject string, payer Payer, d CustomerDetails) (*db.BillingCustomer, error) {
	if s.stripe == nil {
		return nil, ErrNotConfigured
	}
	err := s.authorizeSubject(subject, payer.UserID)
	if err != nil {
		return nil, err
	}
	d.Name, d.Phone = strings.TrimSpace(d.Name), strings.TrimSpace(d.Phone)
	a := &d.Address
	a.Line1, a.Line2, a.PostalCode = strings.TrimSpace(a.Line1), strings.TrimSpace(a.Line2), strings.TrimSpace(a.PostalCode)
	a.City, a.State, a.Country = strings.TrimSpace(a.City), strings.TrimSpace(a.State), strings.ToUpper(strings.TrimSpace(a.Country))
	switch {
	case d.Kind != KindIndividual && d.Kind != KindCompany:
		return nil, fmt.Errorf("%w: kind must be %q or %q", ErrInvalid, KindIndividual, KindCompany)
	case d.Name == "" || a.Line1 == "" || a.PostalCode == "" || a.City == "":
		return nil, fmt.Errorf("%w: name, addressLine1, postalCode and city are required", ErrInvalid)
	case len(a.Country) != 2:
		return nil, fmt.Errorf("%w: country must be a 2-letter ISO code", ErrInvalid)
	}

	customer, err := s.ensureStripeCustomer(ctx, Subject(subject), payer)
	if err != nil {
		return nil, err
	}
	taxID := NormalizeTaxID(d.TaxID, a.Country)
	newTaxID := taxID != "" && customer.TaxID == ""
	switch {
	case customer.TaxID != "" && taxID != "" && taxID != NormalizeTaxID(customer.TaxID, a.Country):
		return nil, fmt.Errorf("%w: the tax id of this account cannot be changed", ErrConflict)
	case taxID == "":
		taxID = NormalizeTaxID(customer.TaxID, a.Country)
	}
	if taxID == "" && d.Kind == KindCompany {
		return nil, fmt.Errorf("%w: a company needs its tax id (CIF)", ErrInvalid)
	}
	if taxID != "" {
		err = ValidateTaxID(taxID, d.Kind, a.Country)
		if err != nil {
			return nil, err
		}
		if newTaxID && s.taxChecker != nil && a.Country != "ES" {
			valid, cerr := s.taxChecker.Check(ctx, a.Country, taxID)
			switch {
			case cerr != nil:
				log.Printf("billing: tax id %s not verified, accepted: %v", taxID, cerr)
			case !valid:
				return nil, fmt.Errorf("%w: tax id %q is not registered as an EU VAT number", ErrInvalid, taxID)
			}
		}
	}

	update := StripeCustomerUpdate{Name: d.Name, Phone: d.Phone, Address: d.Address}
	if newTaxID && a.Country == "ES" {
		update.TaxID = taxID
	}
	err = s.stripe.UpdateCustomer(ctx, customer.StripeCustomerID, update)
	if err != nil {
		return nil, err
	}

	customer.Kind, customer.Name, customer.TaxID, customer.Phone = d.Kind, d.Name, taxID, d.Phone
	customer.AddressLine1, customer.AddressLine2, customer.PostalCode = a.Line1, a.Line2, a.PostalCode
	customer.City, customer.State, customer.Country = a.City, a.State, a.Country
	err = s.repo.SaveCustomer(customer)
	if err != nil {
		return nil, err
	}
	if s.odoo != nil {
		// The next invoice calls ensurePartner again, so a failure here only delays the update.
		_, err = s.ensurePartner(ctx, subject)
		if err != nil {
			log.Printf("billing: Odoo partner of %s not updated, the next invoice will retry: %v", subject, err)
		}
	}
	return customer, nil
}
