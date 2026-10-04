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
