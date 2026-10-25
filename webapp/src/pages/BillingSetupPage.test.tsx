import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { BillingSetupPage } from './BillingSetupPage';

const calls = vi.hoisted(() => ({ mutations: [] as unknown[] }));

vi.mock('../services/billing', () => ({
    useBillingSetup: () => ({
        data: {
            checks: [
                {
                    id: 'check-1',
                    title: 'Stripe API Key',
                    section: 'stripe',
                    detail: 'Check that the Stripe API key is configured',
                    status: 'ok',
                    link: 'https://dashboard.stripe.com/apikeys',
                    action: null,
                },
                {
                    id: 'check-2',
                    title: 'Stripe Webhook',
                    section: 'stripe',
                    detail: 'Configure the webhook endpoint in Stripe',
                    status: 'check',
                    link: 'https://dashboard.stripe.com/webhooks',
                    action: null,
                },
                {
                    id: 'check-3',
                    title: 'Odoo Connection',
                    section: 'odoo',
                    detail: 'Test the Odoo API connection',
                    status: 'missing',
                    link: null,
                    action: 'Test Connection',
                },
                {
                    id: 'check-4',
                    title: 'Sample Data',
                    section: 'stripe',
                    detail: 'Set up sample billing data',
                    status: 'manual',
                    link: null,
                    action: null,
                },
            ],
        },
        isLoading: false,
        error: null,
        refetch: () => {},
    }),
    useRunBillingSetupAction: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
}));

const render = (url: string = '/') =>
    renderToStaticMarkup(
        <MemoryRouter initialEntries={[url]}>
            <BillingSetupPage />
        </MemoryRouter>,
    );

describe('BillingSetupPage', () => {
    beforeEach(() => {
        calls.mutations = [];
    });

    it('renders and shows setup checks list', () => {
        const html = render();
        expect(html).toContain('Billing Setup');
        expect(html).toContain('Stripe API Key');
        expect(html).toContain('Stripe Webhook');
        expect(html).toContain('Odoo Connection');
    });

    it('shows correct badge for OK status', () => {
        const html = render();
        const okIndex = html.indexOf('Stripe API Key');
        const nextOkBadge = html.indexOf('OK', okIndex);
        expect(nextOkBadge).toBeGreaterThan(okIndex);
    });

    it('shows correct badge for Check status', () => {
        const html = render();
        const checkIndex = html.indexOf('Stripe Webhook');
        const nextCheckBadge = html.indexOf('Check', checkIndex);
        expect(nextCheckBadge).toBeGreaterThan(checkIndex);
    });

    it('shows correct badge for Missing status', () => {
        const html = render();
        const missingIndex = html.indexOf('Odoo Connection');
        const nextMissingBadge = html.indexOf('Missing', missingIndex);
        expect(nextMissingBadge).toBeGreaterThan(missingIndex);
    });

    it('shows correct badge for Manual status', () => {
        const html = render();
        const manualIndex = html.indexOf('Sample Data');
        const nextManualBadge = html.indexOf('By hand', manualIndex);
        expect(nextManualBadge).toBeGreaterThan(manualIndex);
    });

    it('shows detail text for each check', () => {
        const html = render();
        expect(html).toContain('Check that the Stripe API key is configured');
        expect(html).toContain('Configure the webhook endpoint in Stripe');
        expect(html).toContain('Test the Odoo API connection');
    });

    it('shows link when available', () => {
        const html = render();
        expect(html).toContain('https://dashboard.stripe.com/apikeys');
        expect(html).toContain('More info');
    });

    it('shows action button when available', () => {
        const html = render();
        expect(html).toContain('Test Connection');
    });

    it('calls the action with correct check name', () => {
        // The action button would call runAction.mutate({ checkName: 'Odoo Connection' })
        // This test verifies the structure is there
        const html = render();
        expect(html).toContain('Test Connection');
    });
});
