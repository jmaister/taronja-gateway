import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { RateLimiterStatsPage } from './RateLimiterStatsPage';

const calls = vi.hoisted(() => ({ filters: [] as unknown[] }));

const blocked = (ipAddress: string, country?: string) => ({
    id: ipAddress,
    ipAddress,
    country,
    reason: 'rate_limit',
    triggerCount: 1,
    blockedAt: '2026-10-01T10:00:00Z',
    blockedUntil: '2026-10-01T11:00:00Z',
});

vi.mock('../services/rateLimiter', () => ({
    useRateLimiterStats: () => ({ data: [], isLoading: false, error: null, refetch: () => {}, dataUpdatedAt: 0 }),
    useBlockedClients: (filter: { country?: string }) => {
        calls.filters.push(filter);
        const items = filter.country ? [blocked('203.0.113.2', 'Spain')] : [blocked('203.0.113.1'), blocked('203.0.113.2', 'Spain')];
        return { data: { items, totalCount: items.length }, isLoading: false, error: null };
    },
}));
vi.mock('../components/LazyBlockedClientsWorldMap', () => ({ LazyBlockedClientsWorldMap: () => <div>attacker-map</div> }));

const render = (url: string) =>
    renderToStaticMarkup(
        <MemoryRouter initialEntries={[url]}>
            <RateLimiterStatsPage />
        </MemoryRouter>,
    );

beforeEach(() => {
    calls.filters = [];
});

describe('RateLimiterStatsPage country filter', () => {
    it('lists every block and offers the countries as filters', () => {
        const html = render('/');
        expect(html).toContain('203.0.113.1');
        expect(html).toContain('title="Filter by Spain"');
        expect(html).not.toContain('Country: Spain');
    });

    it('filters the history by the country in the URL and shows how to clear it', () => {
        const html = render('/?country=Spain');
        expect(calls.filters).toContainEqual({ country: 'Spain' });
        expect(calls.filters).toContainEqual({});
        expect(html).not.toContain('203.0.113.1');
        expect(html).toContain('Country: Spain');
        expect(html).toContain('title="Clear filter: Spain"');
    });
});
