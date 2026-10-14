import { timePeriods } from '../components/StatisticsDateRange';
import { requestTypeFilterToIsStatic } from '../components/RequestTypeFilter';
import type { RequestTypeFilterValue } from '../components/RequestTypeFilter';

export interface TrafficFilterState {
    startDate: string;
    endDate: string;
    requestType: RequestTypeFilterValue;
    statusCode: string;
    country: string;
    deviceType: string;
    platform: string;
    browser: string;
    username: string;
    fingerprint: string;
    fingerprintType: string;
}

export type TrafficDimensionKey = Exclude<
    keyof TrafficFilterState,
    'startDate' | 'endDate' | 'requestType'
>;

export interface TrafficDimensionField {
    key: TrafficDimensionKey;
    label: string;
    placeholder: string;
}

export const trafficDimensionFields: TrafficDimensionField[] = [
    { key: 'statusCode', label: 'Status code', placeholder: '200' },
    { key: 'country', label: 'Country', placeholder: 'Spain' },
    { key: 'deviceType', label: 'Device type', placeholder: 'Desktop' },
    { key: 'platform', label: 'Platform', placeholder: 'Windows' },
    { key: 'browser', label: 'Browser', placeholder: 'Chrome' },
    { key: 'username', label: 'User', placeholder: 'guest' },
    { key: 'fingerprint', label: 'Fingerprint', placeholder: 'stable:…' },
    { key: 'fingerprintType', label: 'Fingerprint type', placeholder: 'stable' },
];

export function startOfDayUTC(date: string): string {
    return `${date}T00:00:00Z`;
}

export function endOfDayUTC(date: string): string {
    return `${date}T23:59:59Z`;
}

function textOrUndefined(value: string): string | undefined {
    const trimmed = value.trim();
    return trimmed === '' ? undefined : trimmed;
}

export interface TrafficFilterQuery {
    start_date: string;
    end_date: string;
    is_static?: boolean;
    status_code?: number;
    country?: string;
    device_type?: string;
    platform?: string;
    browser?: string;
    username?: string;
    fingerprint?: string;
    fingerprint_type?: string;
}

export function trafficFilterToQuery(filter: TrafficFilterState): TrafficFilterQuery {
    const status = Number.parseInt(filter.statusCode, 10);
    return {
        start_date: startOfDayUTC(filter.startDate),
        end_date: endOfDayUTC(filter.endDate),
        is_static: requestTypeFilterToIsStatic(filter.requestType),
        status_code: Number.isNaN(status) ? undefined : status,
        country: textOrUndefined(filter.country),
        device_type: textOrUndefined(filter.deviceType),
        platform: textOrUndefined(filter.platform),
        browser: textOrUndefined(filter.browser),
        username: textOrUndefined(filter.username),
        fingerprint: textOrUndefined(filter.fingerprint),
        fingerprint_type: textOrUndefined(filter.fingerprintType),
    };
}

export function defaultTrafficFilter(): TrafficFilterState {
    const range = timePeriods[0].getDateRange();
    return {
        startDate: range.startDate,
        endDate: range.endDate,
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
}

const FILTER_KEYS = Object.keys(defaultTrafficFilter()) as (keyof TrafficFilterState)[];

export function filterToSearchParams(filter: TrafficFilterState): URLSearchParams {
    const params = new URLSearchParams();
    for (const key of FILTER_KEYS) {
        const value = filter[key];
        if (value !== '' && !(key === 'requestType' && value === 'all')) params.set(key, value);
    }
    return params;
}

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const REQUEST_TYPES = ['all', 'static', 'dynamic'];

// Returns null when the params carry no filter at all (so callers can fall back).
export function filterFromSearchParams(params: URLSearchParams): TrafficFilterState | null {
    if (!FILTER_KEYS.some((k) => params.has(k))) return null;
    const filter = defaultTrafficFilter();
    for (const key of FILTER_KEYS) {
        const value = params.get(key);
        if (value === null) continue;
        if ((key === 'startDate' || key === 'endDate') && !DATE_RE.test(value)) continue;
        if (key === 'requestType' && !REQUEST_TYPES.includes(value)) continue;
        (filter as unknown as Record<string, string>)[key] = value;
    }
    return filter;
}

export function activeDimensionCount(filter: TrafficFilterState): number {
    return trafficDimensionFields.filter((f) => filter[f.key].trim() !== '').length
        + (filter.requestType !== 'all' ? 1 : 0);
}

export function isDefaultFilter(filter: TrafficFilterState): boolean {
    return activeDimensionCount(filter) === 0;
}
