import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { SubscriptionsPage } from './SubscriptionsPage';

const calls = vi.hoisted(() => ({ mutations: [] as unknown[] }));

vi.mock('../services/billing', () => ({
    useBillingSubscriptions: () => ({
        data: [
            {
                id: 'sub-1',
                subject: 'user:123',
                planKey: 'academia-monthly',
                status: 'active',
                currentPeriodEnd: '2026-11-25T00:00:00Z',
                trialEnd: null,
                cancelAtPeriodEnd: false,
                graceUntil: null,
            },
            {
                id: 'sub-2',
                subject: 'org:456',
                planKey: 'academia-yearly',
                status: 'trialing',
                currentPeriodEnd: '2027-01-01T00:00:00Z',
                trialEnd: '2026-11-01T00:00:00Z',
                cancelAtPeriodEnd: false,
                graceUntil: null,
            },
        ],
        isLoading: false,
        error: null,
        refetch: () => {},
    }),
    useActivateBillingSubscription: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
    useCancelBillingSubscription: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
}));

const render = (url: string = '/') =>
    renderToStaticMarkup(
        <MemoryRouter initialEntries={[url]}>
            <SubscriptionsPage />
        </MemoryRouter>,
    );

describe('SubscriptionsPage', () => {
    beforeEach(() => {
        calls.mutations = [];
    });

    it('renders and shows subscriptions list', () => {
        const html = render();
        expect(html).toContain('Subscriptions');
        expect(html).toContain('user:123');
        expect(html).toContain('org:456');
    });

    it('shows subscription status badges', () => {
        const html = render();
        expect(html).toContain('active');
        expect(html).toContain('trialing');
    });

    it('shows plans', () => {
        const html = render();
        expect(html).toContain('academia-monthly');
        expect(html).toContain('academia-yearly');
    });

    it('shows cancel button for active subscriptions', () => {
        const html = render();
        expect(html).toContain('Cancel');
    });
});
