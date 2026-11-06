// Package stripeapi implements billing.Stripe with stripe-go/v87, which pins
// Stripe API version 2026-09-30.endive.
package stripeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/jmaister/taronja-gateway/billing"
	"github.com/stripe/stripe-go/v87"
)

// Client talks to Stripe.
type Client struct {
	sc            *stripe.Client
	webhookSecret string
}

// Option configures a Client.
type Option func(*clientOptions)

type clientOptions struct {
	apiURL string
}

// WithAPIURL sends every request to url instead of api.stripe.com
// (stripe-mock, httptest).
func WithAPIURL(url string) Option {
	return func(o *clientOptions) { o.apiURL = url }
}

// New returns a Client for the secret key; webhookSecret verifies webhooks.
func New(secretKey, webhookSecret string, opts ...Option) *Client {
	var o clientOptions
	httpClient := &http.Client{Timeout: 80 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}
	var clientOpts []stripe.ClientOption
	if o.apiURL != "" {
		maxRetries := int64(0)
		cfg := &stripe.BackendConfig{URL: stripe.String(o.apiURL), HTTPClient: httpClient, MaxNetworkRetries: &maxRetries}
		clientOpts = append(clientOpts, stripe.WithBackends(stripe.NewBackendsWithConfig(cfg)))
	} else {
		cfg := &stripe.BackendConfig{HTTPClient: httpClient}
		clientOpts = append(clientOpts, stripe.WithBackends(stripe.NewBackendsWithConfig(cfg)))
	}
	return &Client{sc: stripe.NewClient(secretKey, clientOpts...), webhookSecret: webhookSecret}
}

var _ billing.Stripe = (*Client)(nil)

func wrap(err error) error {
	return fmt.Errorf("stripe: %w", err)
}

func (c *Client) EnsureProduct(ctx context.Context, existingID, name, planKey, statementDescriptor string) (string, error) {
	if existingID == "" {
		params := &stripe.ProductCreateParams{Name: stripe.String(name)}
		if statementDescriptor != "" {
			params.StatementDescriptor = stripe.String(statementDescriptor)
		}
		params.AddMetadata("tg_plan", planKey)
		product, err := c.sc.V1Products.Create(ctx, params)
		if err != nil {
			return "", wrap(err)
		}
		return product.ID, nil
	}
	update := &stripe.ProductUpdateParams{Name: stripe.String(name)}
	if statementDescriptor != "" {
		update.StatementDescriptor = stripe.String(statementDescriptor)
	}
	product, err := c.sc.V1Products.Update(ctx, existingID, update)
	if err != nil {
		return "", wrap(err)
	}
	return product.ID, nil
}

func (c *Client) CreatePrice(ctx context.Context, p billing.StripePriceParams) (string, error) {
	params := &stripe.PriceCreateParams{
		Product:     stripe.String(p.ProductID),
		UnitAmount:  stripe.Int64(p.AmountCents),
		Currency:    stripe.String(p.Currency),
		TaxBehavior: stripe.String(string(stripe.PriceTaxBehaviorInclusive)),
	}
	if p.Nickname != "" {
		params.Nickname = stripe.String(p.Nickname)
	}
	if p.LookupKey != "" {
		params.LookupKey = stripe.String(p.LookupKey)
		params.TransferLookupKey = stripe.Bool(true)
	}
	if p.Interval != "" {
		params.Recurring = &stripe.PriceCreateRecurringParams{Interval: stripe.String(p.Interval)}
	}
	price, err := c.sc.V1Prices.Create(ctx, params)
	if err != nil {
		return "", wrap(err)
	}
	return price.ID, nil
}

func (c *Client) ArchivePrice(ctx context.Context, priceID string) error {
	_, err := c.sc.V1Prices.Update(ctx, priceID, &stripe.PriceUpdateParams{Active: stripe.Bool(false)})
	if isMissing(err) {
		return nil
	}
	if err != nil {
		return wrap(err)
	}
	return nil
}

func (c *Client) EnsureCustomer(ctx context.Context, existingID, subject, name, email string) (string, error) {
	if existingID == "" {
		params := &stripe.CustomerCreateParams{Name: stripe.String(name), Email: stripe.String(email)}
		params.AddMetadata("tg_subject", subject)
		customer, err := c.sc.V1Customers.Create(ctx, params)
		if err != nil {
			return "", wrap(err)
		}
		return customer.ID, nil
	}
	params := &stripe.CustomerUpdateParams{Name: stripe.String(name), Email: stripe.String(email)}
	customer, err := c.sc.V1Customers.Update(ctx, existingID, params)
	if err != nil {
		return "", wrap(err)
	}
	return customer.ID, nil
}

func (c *Client) UpdateCustomer(ctx context.Context, customerID string, u billing.StripeCustomerUpdate) error {
	params := &stripe.CustomerUpdateParams{
		Name: stripe.String(u.Name), Phone: stripe.String(u.Phone),
		Address: &stripe.AddressParams{
			Line1: stripe.String(u.Address.Line1), Line2: stripe.String(u.Address.Line2),
			PostalCode: stripe.String(u.Address.PostalCode), City: stripe.String(u.Address.City),
			State: stripe.String(u.Address.State), Country: stripe.String(u.Address.Country),
		},
	}
	_, err := c.sc.V1Customers.Update(ctx, customerID, params)
	if err != nil {
		return wrap(err)
	}
	if u.TaxID == "" {
		return nil
	}
	_, err = c.sc.V1TaxIDs.Create(ctx, &stripe.TaxIDCreateParams{Customer: stripe.String(customerID), Type: stripe.String("es_cif"), Value: stripe.String(u.TaxID)})
	if err != nil {
		return wrap(err)
	}
	return nil
}

func (c *Client) CreateCheckoutSession(ctx context.Context, p billing.CheckoutParams) (billing.CheckoutSession, error) {
	params := &stripe.CheckoutSessionCreateParams{
		Customer:                 stripe.String(p.CustomerID),
		Mode:                     stripe.String(p.Mode),
		SuccessURL:               stripe.String(p.SuccessURL),
		CancelURL:                stripe.String(p.CancelURL),
		ClientReferenceID:        stripe.String(p.CheckoutID),
		BillingAddressCollection: stripe.String("required"),
		Locale:                   stripe.String("es"),
		CustomerUpdate: &stripe.CheckoutSessionCreateCustomerUpdateParams{
			Name:    stripe.String("auto"),
			Address: stripe.String("auto"),
		},
		TaxIDCollection: &stripe.CheckoutSessionCreateTaxIDCollectionParams{Enabled: stripe.Bool(true)},
	}
	if p.AskCustomerType {
		params.NameCollection = &stripe.CheckoutSessionCreateNameCollectionParams{
			Business:   &stripe.CheckoutSessionCreateNameCollectionBusinessParams{Enabled: stripe.Bool(true), Optional: stripe.Bool(true)},
			Individual: &stripe.CheckoutSessionCreateNameCollectionIndividualParams{Enabled: stripe.Bool(true), Optional: stripe.Bool(false)},
		}
	}
	if b := p.Brand; b.Name != "" || b.LogoURL != "" || b.ButtonColor != "" || b.BackgroundColor != "" {
		params.BrandingSettings = &stripe.CheckoutSessionCreateBrandingSettingsParams{}
		if b.Name != "" {
			params.BrandingSettings.DisplayName = stripe.String(b.Name)
		}
		if b.ButtonColor != "" {
			params.BrandingSettings.ButtonColor = stripe.String(b.ButtonColor)
		}
		if b.BackgroundColor != "" {
			params.BrandingSettings.BackgroundColor = stripe.String(b.BackgroundColor)
		}
		if b.LogoURL != "" {
			params.BrandingSettings.Logo = &stripe.CheckoutSessionCreateBrandingSettingsLogoParams{Type: stripe.String("url"), URL: stripe.String(b.LogoURL)}
		}
	}
	if p.CollectShipping {
		params.CustomerUpdate.Shipping = stripe.String("auto")
		params.ShippingAddressCollection = &stripe.CheckoutSessionCreateShippingAddressCollectionParams{
			AllowedCountries: []*string{stripe.String("ES")},
		}
	}
	for _, line := range p.Lines {
		params.LineItems = append(params.LineItems, &stripe.CheckoutSessionCreateLineItemParams{
			Price:    stripe.String(line.PriceID),
			Quantity: stripe.Int64(line.Quantity),
		})
	}
	metadata := map[string]string{"tg_checkout": p.CheckoutID}
	for k, v := range p.Metadata {
		metadata[k] = v
	}
	params.Metadata = metadata
	switch p.Mode {
	case "subscription":
		params.SubscriptionData = &stripe.CheckoutSessionCreateSubscriptionDataParams{Metadata: metadata}
		if p.TrialDays > 0 {
			params.SubscriptionData.TrialPeriodDays = stripe.Int64(int64(p.TrialDays))
		}
	case "payment":
		params.PaymentIntentData = &stripe.CheckoutSessionCreatePaymentIntentDataParams{Metadata: metadata}
		if p.StatementDescriptorSuffix != "" {
			params.PaymentIntentData.StatementDescriptorSuffix = stripe.String(p.StatementDescriptorSuffix)
		}
		if p.SaveCard {
			params.PaymentIntentData.SetupFutureUsage = stripe.String("off_session")
		}
	}
	params.SetIdempotencyKey("tg-checkout-" + p.CheckoutID)
	session, err := c.sc.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return billing.CheckoutSession{}, wrap(err)
	}
	return billing.CheckoutSession{ID: session.ID, URL: session.URL}, nil
}

func (c *Client) CreateSubscription(ctx context.Context, p billing.SubscriptionParams) (billing.StripeSubscription, error) {
	paymentMethodID := p.PaymentMethodID
	if paymentMethodID == "" {
		found, err := c.latestCard(ctx, p.CustomerID)
		if err != nil {
			return billing.StripeSubscription{}, err
		}
		paymentMethodID = found
	}
	params := &stripe.SubscriptionCreateParams{
		Customer:             stripe.String(p.CustomerID),
		DefaultPaymentMethod: stripe.String(paymentMethodID),
		Items:                []*stripe.SubscriptionCreateItemParams{{Price: stripe.String(p.PriceID)}},
		Metadata:             p.Metadata,
	}
	if !p.TrialEnd.IsZero() {
		params.TrialEnd = stripe.Int64(p.TrialEnd.Unix())
	}
	if p.IdempotencyKey != "" {
		params.SetIdempotencyKey(p.IdempotencyKey)
	}
	sub, err := c.sc.V1Subscriptions.Create(ctx, params)
	if err != nil {
		return billing.StripeSubscription{}, wrap(err)
	}
	return mapSubscription(sub), nil
}

// latestCard returns the customer's most recently added card.
func (c *Client) latestCard(ctx context.Context, customerID string) (string, error) {
	params := &stripe.CustomerListPaymentMethodsParams{Customer: stripe.String(customerID), Type: stripe.String("card")}
	params.Limit = stripe.Int64(1)
	for pm, err := range c.sc.V1Customers.ListPaymentMethods(ctx, params).All(ctx) {
		if err != nil {
			return "", wrap(err)
		}
		return pm.ID, nil
	}
	return "", fmt.Errorf("stripe: customer %s has no saved card", customerID)
}

func (c *Client) GetSubscription(ctx context.Context, subscriptionID string) (billing.StripeSubscription, error) {
	sub, err := c.sc.V1Subscriptions.Retrieve(ctx, subscriptionID, nil)
	if err != nil {
		return billing.StripeSubscription{}, wrap(err)
	}
	return mapSubscription(sub), nil
}

func (c *Client) CancelSubscription(ctx context.Context, subscriptionID string, atPeriodEnd bool) (billing.StripeSubscription, error) {
	var sub *stripe.Subscription
	var err error
	if atPeriodEnd {
		sub, err = c.sc.V1Subscriptions.Update(ctx, subscriptionID, &stripe.SubscriptionUpdateParams{CancelAtPeriodEnd: stripe.Bool(true)})
	} else {
		sub, err = c.sc.V1Subscriptions.Cancel(ctx, subscriptionID, &stripe.SubscriptionCancelParams{})
	}
	if err != nil {
		return billing.StripeSubscription{}, wrap(err)
	}
	return mapSubscription(sub), nil
}

func (c *Client) EndTrial(ctx context.Context, subscriptionID, idempotencyKey string) (billing.StripeSubscription, error) {
	params := &stripe.SubscriptionUpdateParams{TrialEndNow: stripe.Bool(true)}
	params.SetIdempotencyKey(idempotencyKey)
	sub, err := c.sc.V1Subscriptions.Update(ctx, subscriptionID, params)
	if err != nil {
		return billing.StripeSubscription{}, wrap(err)
	}
	return mapSubscription(sub), nil
}

// portalCustomerUpdates is what a customer may edit in the portal: the
// invoice name and address. Not the tax id (immutable once set) nor the email.
var portalCustomerUpdates = []string{"name", "address"}

// portalProration invoices an upgrade at once (the prorated difference).
// Downgrades never reach it: they are scheduled for the period end, so there
// is no credit or refund and the new price starts with the next cycle.
const portalProration = "always_invoice"

func (c *Client) ConfigurePortal(ctx context.Context, p billing.PortalParams) (string, error) {
	var products []*stripe.BillingPortalConfigurationCreateFeaturesSubscriptionUpdateProductParams
	for product, prices := range p.SwitchProducts {
		products = append(products, &stripe.BillingPortalConfigurationCreateFeaturesSubscriptionUpdateProductParams{
			Product: stripe.String(product),
			Prices:  stripe.StringSlice(prices),
		})
	}
	if p.ConfigID != "" {
		cfg, err := c.updatePortal(ctx, p, products)
		if err == nil {
			return cfg, nil
		}
		if !isMissing(err) {
			return "", wrap(err)
		}
	}
	features := &stripe.BillingPortalConfigurationCreateFeaturesParams{
		PaymentMethodUpdate: &stripe.BillingPortalConfigurationCreateFeaturesPaymentMethodUpdateParams{Enabled: stripe.Bool(true)},
		SubscriptionCancel: &stripe.BillingPortalConfigurationCreateFeaturesSubscriptionCancelParams{
			Enabled: stripe.Bool(true),
			Mode:    stripe.String("at_period_end"),
		},
		InvoiceHistory: &stripe.BillingPortalConfigurationCreateFeaturesInvoiceHistoryParams{Enabled: stripe.Bool(false)},
		CustomerUpdate: &stripe.BillingPortalConfigurationCreateFeaturesCustomerUpdateParams{
			Enabled:        stripe.Bool(true),
			AllowedUpdates: stripe.StringSlice(portalCustomerUpdates),
		},
		SubscriptionUpdate: &stripe.BillingPortalConfigurationCreateFeaturesSubscriptionUpdateParams{
			Enabled: stripe.Bool(len(products) > 0),
		},
	}
	if len(products) > 0 {
		features.SubscriptionUpdate.DefaultAllowedUpdates = stripe.StringSlice([]string{"price"})
		features.SubscriptionUpdate.ProrationBehavior = stripe.String(portalProration)
		features.SubscriptionUpdate.ScheduleAtPeriodEnd = &stripe.BillingPortalConfigurationCreateFeaturesSubscriptionUpdateScheduleAtPeriodEndParams{
			Conditions: []*stripe.BillingPortalConfigurationCreateFeaturesSubscriptionUpdateScheduleAtPeriodEndConditionParams{
				{Type: stripe.String("decreasing_item_amount")}, {Type: stripe.String("shortening_interval")},
			},
		}
		features.SubscriptionUpdate.Products = products
	}
	params := &stripe.BillingPortalConfigurationCreateParams{
		Features:         features,
		DefaultReturnURL: stripe.String(p.ReturnURL),
	}
	cfg, err := c.sc.V1BillingPortalConfigurations.Create(ctx, params)
	if err != nil {
		return "", wrap(err)
	}
	return cfg.ID, nil
}

func (c *Client) updatePortal(ctx context.Context, p billing.PortalParams, products []*stripe.BillingPortalConfigurationCreateFeaturesSubscriptionUpdateProductParams) (string, error) {
	update := &stripe.BillingPortalConfigurationUpdateFeaturesSubscriptionUpdateParams{Enabled: stripe.Bool(len(products) > 0)}
	if len(products) > 0 {
		update.DefaultAllowedUpdates = stripe.StringSlice([]string{"price"})
		update.ProrationBehavior = stripe.String(portalProration)
		update.ScheduleAtPeriodEnd = &stripe.BillingPortalConfigurationUpdateFeaturesSubscriptionUpdateScheduleAtPeriodEndParams{
			Conditions: []*stripe.BillingPortalConfigurationUpdateFeaturesSubscriptionUpdateScheduleAtPeriodEndConditionParams{
				{Type: stripe.String("decreasing_item_amount")}, {Type: stripe.String("shortening_interval")},
			},
		}
		for _, prod := range products {
			update.Products = append(update.Products, &stripe.BillingPortalConfigurationUpdateFeaturesSubscriptionUpdateProductParams{
				Product: prod.Product, Prices: prod.Prices,
			})
		}
	}
	params := &stripe.BillingPortalConfigurationUpdateParams{
		Features: &stripe.BillingPortalConfigurationUpdateFeaturesParams{
			SubscriptionUpdate: update,
			CustomerUpdate: &stripe.BillingPortalConfigurationUpdateFeaturesCustomerUpdateParams{
				Enabled:        stripe.Bool(true),
				AllowedUpdates: stripe.StringSlice(portalCustomerUpdates),
			},
		},
		DefaultReturnURL: stripe.String(p.ReturnURL),
	}
	cfg, err := c.sc.V1BillingPortalConfigurations.Update(ctx, p.ConfigID, params)
	if err != nil {
		return "", err
	}
	return cfg.ID, nil
}

// isMissing reports whether err is Stripe saying the resource does not
// exist (a configuration of the other mode, or deleted).
func isMissing(err error) bool {
	var se *stripe.Error
	return errors.As(err, &se) && se.Code == stripe.ErrorCodeResourceMissing
}

func (c *Client) GetPortalConfig(ctx context.Context, id string) (billing.PortalConfig, error) {
	cfg, err := c.sc.V1BillingPortalConfigurations.Retrieve(ctx, id, nil)
	if isMissing(err) {
		return billing.PortalConfig{}, fmt.Errorf("stripe: %w: portal configuration %s", billing.ErrNotFound, id)
	}
	if err != nil {
		return billing.PortalConfig{}, wrap(err)
	}
	return billing.PortalConfig{ID: cfg.ID, Active: cfg.Active, ReturnURL: cfg.DefaultReturnURL}, nil
}

// PaymentMethodTypes reads the default payment method configuration: the
// methods whose value is on, as Checkout offers them.
func (c *Client) PaymentMethodTypes(ctx context.Context) ([]string, error) {
	params := &stripe.PaymentMethodConfigurationListParams{Active: stripe.Bool(true)}
	for pmc, err := range c.sc.V1PaymentMethodConfigurations.List(ctx, params).All(ctx) {
		if err != nil {
			return nil, wrap(err)
		}
		if pmc.IsDefault {
			return enabledPaymentMethods(pmc)
		}
	}
	return nil, fmt.Errorf("stripe: no default payment method configuration")
}

func enabledPaymentMethods(pmc *stripe.PaymentMethodConfiguration) ([]string, error) {
	raw, err := json.Marshal(pmc)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	var types []string
	for name, value := range fields {
		var method struct {
			Available         bool `json:"available"`
			DisplayPreference struct {
				Value string `json:"value"`
			} `json:"display_preference"`
		}
		if json.Unmarshal(value, &method) != nil || !method.Available || method.DisplayPreference.Value != "on" {
			continue
		}
		types = append(types, name)
	}
	sort.Strings(types)
	return types, nil
}

func (c *Client) PortalURL(ctx context.Context, customerID, returnURL, configID string) (string, error) {
	params := &stripe.BillingPortalSessionCreateParams{Customer: stripe.String(customerID), ReturnURL: stripe.String(returnURL)}
	if configID != "" {
		params.Configuration = stripe.String(configID)
	}
	session, err := c.sc.V1BillingPortalSessions.Create(ctx, params)
	if err != nil {
		return "", wrap(err)
	}
	return session.URL, nil
}

func (c *Client) Refund(ctx context.Context, paymentIntentID string, amountCents int64, idempotencyKey string) error {
	params := &stripe.RefundCreateParams{PaymentIntent: stripe.String(paymentIntentID)}
	if amountCents > 0 {
		params.Amount = stripe.Int64(amountCents)
	}
	if idempotencyKey != "" {
		params.SetIdempotencyKey(idempotencyKey)
	}
	_, err := c.sc.V1Refunds.Create(ctx, params)
	var se *stripe.Error
	if errors.As(err, &se) && se.Code == stripe.ErrorCodeIdempotencyKeyInUse {
		return fmt.Errorf("%w: a refund of this payment is already in progress", billing.ErrConflict)
	}
	if err != nil {
		return wrap(err)
	}
	return nil
}

// invoiceOfPaymentIntent returns the invoice a payment intent paid, "" when
// it paid none (a Checkout payment in payment mode).
func (c *Client) invoiceOfPaymentIntent(ctx context.Context, paymentIntentID string) (string, error) {
	params := &stripe.InvoicePaymentListParams{
		Payment: &stripe.InvoicePaymentListPaymentParams{Type: stripe.String("payment_intent"), PaymentIntent: stripe.String(paymentIntentID)},
	}
	params.Limit = stripe.Int64(1)
	for ip, err := range c.sc.V1InvoicePayments.List(ctx, params).All(ctx) {
		if err != nil {
			return "", wrap(err)
		}
		if ip.Invoice != nil {
			return ip.Invoice.ID, nil
		}
	}
	return "", nil
}

func mapSubscription(sub *stripe.Subscription) billing.StripeSubscription {
	out := billing.StripeSubscription{
		ID:                sub.ID,
		Status:            string(sub.Status),
		CancelAtPeriodEnd: sub.CancelAtPeriodEnd,
		Metadata:          sub.Metadata,
	}
	if sub.Customer != nil {
		out.CustomerID = sub.Customer.ID
	}
	if sub.TrialEnd != 0 {
		t := time.Unix(sub.TrialEnd, 0).UTC()
		out.TrialEnd = &t
	}
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		item := sub.Items.Data[0]
		if item.Price != nil {
			out.PriceID = item.Price.ID
		}
		if item.CurrentPeriodEnd != 0 {
			out.CurrentPeriodEnd = time.Unix(item.CurrentPeriodEnd, 0).UTC()
		}
	}
	return out
}

var errNoSignature = errors.New("missing signature")
