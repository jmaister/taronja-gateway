import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { TrafficFilterBar } from './TrafficFilterBar';
import { defaultTrafficFilter } from '../lib/trafficFilter';
import type { TrafficFilterController } from '../lib/useTrafficFilter';

const overview = vi.hoisted(() => ({ current: undefined as unknown }));

vi.mock('../services/statistics', () => ({
    useRequestStatistics: () => ({ data: overview.current }),
}));

function controller(overrides: Partial<ReturnType<typeof defaultTrafficFilter>> = {}): TrafficFilterController {
    return {
        filter: { ...defaultTrafficFilter(), ...overrides },
        period: 'last7days',
        setPeriod: () => {},
        setDateRange: () => {},
        update: () => {},
        clearDimensions: () => {},
    } as unknown as TrafficFilterController;
}

describe('TrafficFilterBar', () => {
    it('offers suggestions from the summary breakdowns, skipping blanks', () => {
        overview.current = { requestsByCountry: { ES: 3, US: 1, '': 2 } };
        const html = renderToStaticMarkup(<TrafficFilterBar controller={controller()} />);
        expect(html).toContain('<option value="ES"');
        expect(html).toContain('<option value="US"');
        expect(html).not.toContain('<option value=""');
    });

    it('shows the clear button only when a dimension filter is active', () => {
        overview.current = undefined;
        expect(renderToStaticMarkup(<TrafficFilterBar controller={controller()} />)).not.toContain('Clear filters');
        expect(renderToStaticMarkup(<TrafficFilterBar controller={controller({ country: 'ES' })} />)).toContain('Clear filters (1)');
    });

    it('warns about unrecorded static assets unless the filter is dynamic-only', () => {
        overview.current = { staticAssetsExcluded: true };
        expect(renderToStaticMarkup(<TrafficFilterBar controller={controller({ requestType: 'static' })} />)).toContain('not being recorded');
        expect(renderToStaticMarkup(<TrafficFilterBar controller={controller({ requestType: 'dynamic' })} />)).not.toContain('not being recorded');
    });
});
