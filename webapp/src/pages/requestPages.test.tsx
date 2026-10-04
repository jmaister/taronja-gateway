import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { RequestSummaryPage } from './RequestSummaryPage';
import { RequestsDetailsPage } from './RequestsDetailsPage';

type QueryResult = { data?: unknown; isLoading: boolean; error: unknown; refetch: () => void };

const state = vi.hoisted(() => ({
    summary: undefined as unknown,
    series: undefined as unknown,
    details: undefined as unknown,
}));

vi.mock('../services/statistics', () => ({
    useRequestStatistics: () => state.summary,
    useRequestTimeSeries: () => state.series,
    useRequestDetails: () => state.details,
}));
vi.mock('../components/charts/TimeSeriesChart', () => ({ TimeSeriesChart: () => <div>chart</div> }));
vi.mock('../components/LazyRequestsWorldMap', () => ({ LazyRequestsWorldMap: () => <div>world-map</div> }));
vi.mock('../components/RequestsDetailsTable', () => ({
    RequestsDetailsTable: ({ requests }: { requests: unknown[] }) => <div>{requests.length} requests in table</div>,
}));

const result = (over: Partial<QueryResult>): QueryResult => ({
    data: undefined,
    isLoading: false,
    error: null,
    refetch: () => {},
    ...over,
});

const render = (ui: React.ReactElement) => renderToStaticMarkup(<MemoryRouter>{ui}</MemoryRouter>);

const stats = (over: object = {}) => ({
    totalRequests: 10,
    averageResponseTime: 12.5,
    averageResponseSize: 2048,
    requestsByStatus: { '200': 8, '404': 1, '500': 1 },
    requestsByCountry: { ES: 10 },
    requestsByDeviceType: {},
    requestsByPlatform: {},
    requestsByBrowser: {},
    requestsByFingerprint: {},
    requestsByFingerprintType: {},
    ...over,
});

beforeEach(() => {
    state.summary = result({});
    state.series = result({ data: { points: [] } });
    state.details = result({});
});

describe('RequestSummaryPage', () => {
    it('shows the loading text', () => {
        state.summary = result({ isLoading: true });
        expect(render(<RequestSummaryPage />)).toContain('Loading request summary...');
    });

    it('shows the error with a retry button', () => {
        state.summary = result({ error: new Error('boom') });
        const html = render(<RequestSummaryPage />);
        expect(html).toContain('Error Loading Request Summary');
        expect(html).toContain('boom');
        expect(html).toContain('Retry');
    });

    it('explains an empty result instead of rendering zeros', () => {
        state.summary = result({ data: stats({ totalRequests: 0 }) });
        const html = render(<RequestSummaryPage />);
        expect(html).toContain('No requests match the current filters');
        expect(html).not.toContain('Success Rate');
    });

    it('renders the cards and breakdown tables, with the success rate from 2xx/3xx', () => {
        state.summary = result({ data: stats() });
        const html = render(<RequestSummaryPage />);
        expect(html).toContain('Total Requests');
        expect(html).toContain('80.0%');
        expect(html).toContain('Requests by Country');
    });
});

describe('RequestsDetailsPage', () => {
    it('shows the error with a retry button', () => {
        state.details = result({ error: new Error('nope') });
        const html = render(<RequestsDetailsPage />);
        expect(html).toContain('Error Loading Request Details');
        expect(html).toContain('Retry');
        expect(html).not.toContain('world-map');
    });

    it('passes the requests to the map and the table', () => {
        state.details = result({ data: { requests: [{ id: 1 }, { id: 2 }] } });
        const html = render(<RequestsDetailsPage />);
        expect(html).toContain('world-map');
        expect(html).toContain('2 requests in table');
    });

    it('renders an empty table when there is no data yet', () => {
        expect(render(<RequestsDetailsPage />)).toContain('0 requests in table');
    });
});
