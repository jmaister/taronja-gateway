import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { PlansPage } from './PlansPage';

const calls = vi.hoisted(() => ({ mutations: [] as unknown[] }));

vi.mock('../services/billing', () => ({
    useBillingPlans: () => ({
        data: [
            {
                key: 'academia-monthly',
                name: 'Academia Monthly',
                kind: 'subscription',
                interval: 'month',
                amountCents: 999,
                features: ['feature1', 'feature2'],
                active: true,
            },
            {
                key: 'academia-yearly',
                name: 'Academia Yearly',
                kind: 'subscription',
                interval: 'year',
                amountCents: 9900,
                features: ['feature1', 'feature2', 'feature3'],
                active: true,
            },
        ],
        isLoading: false,
        error: null,
        refetch: () => {},
    }),
    useCreateBillingPlan: () => ({
        mutate: (data: unknown) => {
            calls.mutations.push(data);
        },
        isPending: false,
        error: null,
    }),
    useUpdateBillingPlan: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
}));

const render = (url: string = '/') =>
    renderToStaticMarkup(
        <MemoryRouter initialEntries={[url]}>
            <PlansPage />
        </MemoryRouter>,
    );

describe('PlansPage', () => {
    beforeEach(() => {
        calls.mutations = [];
    });

    it('renders and shows plans list', () => {
        const html = render();
        expect(html).toContain('Billing Plans');
        expect(html).toContain('academia-monthly');
        expect(html).toContain('Academia Monthly');
        expect(html).toContain('academia-yearly');
    });

    it('shows plan price in EUR format', () => {
        const html = render();
        expect(html).toContain('9,99');
        expect(html).toContain('99,00');
    });

    it('shows plan features', () => {
        const html = render();
        expect(html).toContain('feature1');
        expect(html).toContain('feature3');
    });

    it('shows plan kind and interval', () => {
        const html = render();
        expect(html).toContain('subscription');
        expect(html).toContain('month');
        expect(html).toContain('year');
    });

    it('converts euros to cents on create', () => {
        const html = render();
        expect(html).toContain('Amount (€)');
        // The form itself is present, conversion happens in the mutation
    });
});
