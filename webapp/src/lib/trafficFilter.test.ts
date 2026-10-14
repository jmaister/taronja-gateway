import { describe, expect, it } from 'vitest';
import {
    type TrafficFilterState,
    activeDimensionCount,
    filterFromSearchParams,
    filterToSearchParams,
    trafficFilterToQuery,
} from './trafficFilter';

const base: TrafficFilterState = {
    startDate: '2024-03-01',
    endDate: '2024-03-02',
    requestType: 'all',
    statusCode: '',
    country: '',
    deviceType: '',
    platform: '',
    browser: '',
    username: '',
    fingerprint: '',
    fingerprintType: '',
};

describe('trafficFilterToQuery', () => {
    it('sends only the date range when nothing else is set', () => {
        expect(trafficFilterToQuery(base)).toEqual({
            start_date: '2024-03-01T00:00:00Z',
            end_date: '2024-03-02T23:59:59Z',
        });
    });

    it('maps every dimension to its query parameter', () => {
        const q = trafficFilterToQuery({
            ...base,
            requestType: 'dynamic',
            statusCode: '404',
            country: ' Spain ',
            deviceType: 'Desktop',
            platform: 'Windows',
            browser: 'Chrome',
            username: 'guest',
            fingerprint: 'stable:abc',
            fingerprintType: 'stable',
        });
        expect(q).toMatchObject({
            is_static: false,
            status_code: 404,
            country: 'Spain',
            device_type: 'Desktop',
            platform: 'Windows',
            browser: 'Chrome',
            username: 'guest',
            fingerprint: 'stable:abc',
            fingerprint_type: 'stable',
        });
    });

    it('ignores a non-numeric status code', () => {
        expect(trafficFilterToQuery({ ...base, statusCode: 'abc' }).status_code).toBeUndefined();
    });
});

describe('filter <-> search params', () => {
    it('round-trips a filter and omits defaults', () => {
        const filter = { ...base, requestType: 'static' as const, country: 'Spain', statusCode: '404' };
        const params = filterToSearchParams(filter);
        expect(params.get('country')).toBe('Spain');
        expect(params.has('browser')).toBe(false);
        expect(filterFromSearchParams(params)).toEqual(filter);
    });

    it('returns null when no filter keys are present', () => {
        expect(filterFromSearchParams(new URLSearchParams('period=today'))).toBeNull();
    });

    it('ignores malformed dates and request types', () => {
        const f = filterFromSearchParams(new URLSearchParams('startDate=nope&requestType=bogus&country=ES'));
        expect(f?.country).toBe('ES');
        expect(f?.requestType).toBe('all');
        expect(f?.startDate).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    });

    it('counts active filters', () => {
        expect(activeDimensionCount(base)).toBe(0);
        expect(activeDimensionCount({ ...base, requestType: 'static', browser: 'Chrome' })).toBe(2);
    });
});
