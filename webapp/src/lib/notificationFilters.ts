import type { AdminNotificationQuery } from '../services/adminNotifications';

// The filters live in the URL, so a search can be shared and survives reloads.
export const FILTER_KEYS = ['from', 'to', 'type', 'user', 'channel', 'status', 'responded', 'q', 'batch'] as const;
export type FilterKey = (typeof FILTER_KEYS)[number];
export type Filters = Record<FilterKey, string>;

export function readFilters(params: URLSearchParams): Filters {
    return Object.fromEntries(FILTER_KEYS.map(key => [key, params.get(key) ?? ''])) as Filters;
}

// toQuery turns the URL filters into API parameters. Dates are whole local
// days: "from" starts at 00:00 and "to" ends at 23:59:59.999.
export function toQuery(filters: Filters, page: number, limit: number): AdminNotificationQuery {
    const query: AdminNotificationQuery = { page, limit };
    if (filters.from) query.start_date = new Date(`${filters.from}T00:00:00`).toISOString();
    if (filters.to) query.end_date = new Date(`${filters.to}T23:59:59.999`).toISOString();
    if (filters.type) query.type = filters.type;
    if (filters.user) query.user = filters.user;
    if (filters.channel) query.channel = filters.channel;
    if (filters.status) query.status = filters.status as AdminNotificationQuery['status'];
    if (filters.responded) query.responded = filters.responded === 'yes';
    if (filters.q) query.q = filters.q;
    if (filters.batch) query.batch_id = filters.batch;
    return query;
}

