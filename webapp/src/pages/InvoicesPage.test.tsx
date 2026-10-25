import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { InvoicesPage } from './InvoicesPage';

const calls = vi.hoisted(() => ({ mutations: [] as unknown[] }));

vi.mock('../services/billing', () => ({
    useBillingDocuments: () => ({
        data: [
            {
                id: 'doc-1',
                kind: 'invoice',
                number: 'INV-001',
                subject: 'user:123',
                amountCents: 9999,
                state: 'sent',
                sentAt: '2026-10-15T10:00:00Z',
                error: null,
                attempts: 1,
            },
            {
                id: 'doc-2',
                kind: 'credit_note',
                number: 'CN-001',
                subject: 'org:456',
                amountCents: -2500,
                state: 'failed',
                sentAt: null,
                error: 'SMTP connection timeout',
                attempts: 3,
            },
        ],
        isLoading: false,
        error: null,
        refetch: () => {},
    }),
    useRetryBillingDocument: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
}));

const render = (url: string = '/') =>
    renderToStaticMarkup(
        <MemoryRouter initialEntries={[url]}>
            <InvoicesPage />
        </MemoryRouter>,
    );

describe('InvoicesPage', () => {
    beforeEach(() => {
        calls.mutations = [];
    });

    it('renders and shows invoices list', () => {
        const html = render();
        expect(html).toContain('Invoices');
        expect(html).toContain('Credit Notes');
        expect(html).toContain('INV-001');
        expect(html).toContain('CN-001');
    });

    it('shows document kind badges', () => {
        const html = render();
        expect(html).toContain('invoice');
        expect(html).toContain('credit_note');
    });

    it('shows document numbers', () => {
        const html = render();
        expect(html).toContain('INV-001');
        expect(html).toContain('CN-001');
    });

    it('shows document amounts in EUR format', () => {
        const html = render();
        expect(html).toContain('99,99');
        expect(html).toContain('-25,00');
    });

    it('shows attempt count', () => {
        const html = render();
        expect(html).toContain('Attempts');
        expect(html).toContain('1');
        expect(html).toContain('3');
    });

    it('shows PDF button', () => {
        const html = render();
        expect(html).toContain('📄');
    });
});
