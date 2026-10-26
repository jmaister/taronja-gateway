import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { NotificationsPage } from './NotificationsPage';
import { readFilters, toQuery } from '../lib/notificationFilters';

const calls = vi.hoisted(() => ({ queries: [] as unknown[] }));

const item = (id: string, title: string, status: string, channels: Record<string, string>) => ({
    notification: {
        id,
        userId: 'u1',
        batchId: `batch-${id}`,
        type: 'trip',
        title,
        body: `Body of ${title}`,
        createdAt: '2026-10-01T10:00:00Z',
        respondedActionId: id === 'n1' ? 'yes' : null,
        respondedVia: id === 'n1' ? 'sms' : null,
    },
    username: 'lucia',
    email: 'lucia@parents.example',
    status,
    channels,
});

vi.mock('../services/adminNotifications', () => ({
    useAdminNotifications: (query: unknown) => {
        calls.queries.push(query);
        return {
            data: {
                items: [item('n1', 'Excursion', 'sent', { sms: 'sent' }), item('n2', 'Fees due', 'failed', { email: 'failed', sms: 'sent' })],
                total: 120,
                page: 2,
                limit: 50,
            },
            isLoading: false,
            error: null,
            refetch: () => {},
        };
    },
    useNotificationTypes: () => ({ data: ['fees', 'trip'] }),
    useNotificationDeliveries: () => ({ data: [], isLoading: false, error: null }),
}));

const render = (url: string) =>
    renderToStaticMarkup(
        <MemoryRouter initialEntries={[url]}>
            <NotificationsPage />
        </MemoryRouter>,
    );

beforeEach(() => {
    calls.queries = [];
});

describe('NotificationsPage', () => {
    it('lists notifications with recipient, channel statuses and answers', () => {
        const html = render('/notifications');
        expect(html).toContain('Excursion');
        expect(html).toContain('lucia@parents.example');
        expect(html).toContain('title="email: failed"');
        expect(html).toContain('title="sms: sent"');
        expect(html).toMatch(/via (<!-- -->)?sms/);
        expect(html).toContain('<option value="trip">trip</option>');
    });

    it('sends the URL filters and page to the API and shows the range', () => {
        const html = render('/notifications?type=trip&user=lucia&status=failed&channel=email&responded=no&q=fees&page=2&limit=50');
        expect(calls.queries).toContainEqual({
            page: 2,
            limit: 50,
            type: 'trip',
            user: 'lucia',
            status: 'failed',
            channel: 'email',
            responded: false,
            q: 'fees',
        });
        expect(html).toContain('Showing 51–100 of 120');
        expect(html).toContain('Page 2 of 3');
        expect(html).toContain('value="lucia"');
    });

    it('falls back to the first page and the default page size', () => {
        render('/notifications?page=-3&limit=7');
        expect(calls.queries).toContainEqual({ page: 1, limit: 50 });
    });
});

describe('toQuery', () => {
    it('turns local days into a full-day date range', () => {
        const query = toQuery(readFilters(new URLSearchParams('from=2026-10-01&to=2026-10-02')), 1, 25);
        expect(query.start_date).toBe(new Date('2026-10-01T00:00:00').toISOString());
        expect(query.end_date).toBe(new Date('2026-10-02T23:59:59.999').toISOString());
        expect(query).not.toHaveProperty('type');
    });

    it('maps the batch and answered filters', () => {
        const query = toQuery(readFilters(new URLSearchParams('batch=b-1&responded=yes')), 3, 100);
        expect(query).toEqual({ page: 3, limit: 100, batch_id: 'b-1', responded: true });
    });
});
