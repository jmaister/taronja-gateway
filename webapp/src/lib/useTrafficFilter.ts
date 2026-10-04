import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { timePeriods } from '../components/StatisticsDateRange';
import type { DateRange } from '../components/StatisticsDateRange';
import {
    defaultTrafficFilter,
    filterFromSearchParams,
    filterToSearchParams,
    isDefaultFilter,
} from './trafficFilter';
import type { TrafficFilterState } from './trafficFilter';

const STORAGE_KEY = 'tg.trafficFilter';

function readStored(): URLSearchParams | null {
    try {
        const raw = sessionStorage.getItem(STORAGE_KEY);
        return raw === null ? null : new URLSearchParams(raw);
    } catch {
        return null;
    }
}

function writeStored(params: URLSearchParams) {
    try {
        sessionStorage.setItem(STORAGE_KEY, params.toString());
    } catch {
        // storage unavailable: the filter just won't carry across pages
    }
}

// The filter lives in the URL query string so it can be shared and survives a
// reload; the last one is also kept in sessionStorage so it follows the user
// between the statistics pages.
export function useTrafficFilter() {
    const [searchParams, setSearchParams] = useSearchParams();
    const [filter, setFilter] = useState<TrafficFilterState>(() => {
        const fromUrl = filterFromSearchParams(searchParams);
        if (fromUrl) return fromUrl;
        const stored = readStored();
        return (stored && filterFromSearchParams(stored)) || defaultTrafficFilter();
    });
    const [period, setPeriod] = useState<string>(() => {
        const p = searchParams.get('period') ?? readStored()?.get('period');
        return p && timePeriods.some((t) => t.value === p) ? p : 'today';
    });

    useEffect(() => {
        const params = filterToSearchParams(filter);
        params.set('period', period);
        writeStored(params);
        setSearchParams(params, { replace: true });
    }, [filter, period, setSearchParams]);

    const setDateRange = useCallback(
        (range: DateRange) => setFilter((f) => ({ ...f, startDate: range.startDate, endDate: range.endDate })),
        [],
    );
    const update = useCallback(
        <K extends keyof TrafficFilterState>(key: K, value: TrafficFilterState[K]) =>
            setFilter((f) => ({ ...f, [key]: value })),
        [],
    );
    const clearDimensions = useCallback(
        () =>
            setFilter((f) => ({
                ...defaultTrafficFilter(),
                startDate: f.startDate,
                endDate: f.endDate,
            })),
        [],
    );

    return { filter, period, setPeriod, setDateRange, update, clearDimensions, isDefault: isDefaultFilter(filter) };
}

export type TrafficFilterController = ReturnType<typeof useTrafficFilter>;
