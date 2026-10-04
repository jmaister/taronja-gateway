import { useCallback, useState } from 'react';
import { timePeriods } from '../components/StatisticsDateRange';
import type { DateRange } from '../components/StatisticsDateRange';
import type { TrafficFilterState } from './trafficFilter';

export function useTrafficFilter() {
    const [period, setPeriod] = useState<string>('today');
    const [filter, setFilter] = useState<TrafficFilterState>(() => {
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
    });

    const setDateRange = useCallback(
        (range: DateRange) => setFilter((f) => ({ ...f, startDate: range.startDate, endDate: range.endDate })),
        [],
    );
    const update = useCallback(
        <K extends keyof TrafficFilterState>(key: K, value: TrafficFilterState[K]) =>
            setFilter((f) => ({ ...f, [key]: value })),
        [],
    );

    return { filter, period, setPeriod, setDateRange, update };
}

export type TrafficFilterController = ReturnType<typeof useTrafficFilter>;
