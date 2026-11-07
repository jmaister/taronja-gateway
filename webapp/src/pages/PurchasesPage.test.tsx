import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { PurchasesPage } from './PurchasesPage';

const calls = vi.hoisted(() => ({ mutations: [] as unknown[] }));

vi.mock('../services/billing', () => ({
    useBillingPurchases: () => ({
        data: [
            {
                id: 'purch-1',
                subject: 'user:123',
                planKey: 'megabox-onetime',
                amountCents: 4999,
                refundedCents: 0,
                status: 'paid',
                paidAt: '2026-10-01T10:00:00Z',
                subscriptionId: null,
                shippingName: null,
                shippingAddress: null,
            },
            {
                id: 'purch-2',
                subject: 'user:456',
                planKey: 'academia-annual',
                amountCents: 9999,
                refundedCents: 2500,
                status: 'paid',
                paidAt: '2026-09-15T14:00:00Z',
                subscriptionId: 'sub-1',
                shippingName: 'John Doe',
                shippingAddress: '123 Main St, City',
            },
        ],
        isLoading: false,
        error: null,
        refetch: () => {},
    }),
    useRefundBillingPurchase: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
}));

const render = (url: string = '/') =>
    renderToStaticMarkup(
        <MemoryRouter initialEntries={[url]}>
            <PurchasesPage />
        </MemoryRouter>,
    );

describe('PurchasesPage', () => {
    beforeEach(() => {
        calls.mutations = [];
    });

    it('renders and shows purchases list', () => {
        const html = render();
        expect(html).toContain('Purchases');
        expect(html).toContain('megabox-onetime');
        expect(html).toContain('academia-annual');
    });

    it('shows purchase amounts in EUR format', () => {
        const html = render();
        expect(html).toContain('49,99');
        expect(html).toContain('99,99');
    });

    it('shows refunded amounts', () => {
        const html = render();
        expect(html).toContain('25,00');
    });

    it('shows status badges', () => {
        const html = render();
        expect(html).toContain('paid');
    });

    it('shows refund button', () => {
        const html = render();
        expect(html).toContain('Refund');
    });
});
