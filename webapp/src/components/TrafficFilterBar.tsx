import { StatisticsDateRange } from './StatisticsDateRange';
import { RequestTypeFilter } from './RequestTypeFilter';
import { Button } from './ui/Button';
import { activeDimensionCount, defaultTrafficFilter, trafficDimensionFields } from '../lib/trafficFilter';
import type { TrafficDimensionKey } from '../lib/trafficFilter';
import type { TrafficFilterController } from '../lib/useTrafficFilter';
import { useRequestStatistics } from '../services/statistics';

// Suggestion source per dimension: the keys of the matching summary breakdown,
// fetched with the date range and request type only so choosing a value does
// not shrink the list of alternatives.
const SUGGESTIONS: Partial<Record<TrafficDimensionKey, 'requestsByStatus' | 'requestsByCountry' | 'requestsByDeviceType' | 'requestsByPlatform' | 'requestsByBrowser' | 'requestsByUser' | 'requestsByFingerprintType'>> = {
    statusCode: 'requestsByStatus',
    country: 'requestsByCountry',
    deviceType: 'requestsByDeviceType',
    platform: 'requestsByPlatform',
    browser: 'requestsByBrowser',
    username: 'requestsByUser',
    fingerprintType: 'requestsByFingerprintType',
};

export function TrafficFilterBar({ controller }: { controller: TrafficFilterController }) {
    const { filter, period, setPeriod, setDateRange, update, clearDimensions } = controller;
    const { startDate, endDate, requestType } = filter;
    const { data: overview } = useRequestStatistics({
        ...defaultTrafficFilter(),
        startDate,
        endDate,
        requestType,
    });
    const active = activeDimensionCount(filter);

    return (
        <div className="space-y-3">
            <div className="flex flex-wrap items-end gap-4">
                <StatisticsDateRange
                    dateRange={{ startDate, endDate }}
                    setDateRange={setDateRange}
                    selectedPeriod={period}
                    setSelectedPeriod={setPeriod}
                />
                <RequestTypeFilter value={requestType} onChange={(v) => update('requestType', v)} />
                {active > 0 && (
                    <Button variant="secondary" size="sm" onClick={clearDimensions}>
                        Clear filters ({active})
                    </Button>
                )}
            </div>
            <div className="flex flex-wrap items-end gap-4">
                {trafficDimensionFields.map((field) => {
                    const source = SUGGESTIONS[field.key];
                    const options = source && overview ? Object.keys(overview[source] ?? {}).filter((k) => k.trim() !== '') : [];
                    const listId = `traffic-filter-${field.key}-options`;
                    return (
                        <div key={field.key}>
                            <label className="block text-sm font-medium text-muted-fg" htmlFor={`traffic-filter-${field.key}`}>
                                {field.label}
                            </label>
                            <input
                                id={`traffic-filter-${field.key}`}
                                className="tg-input py-1.5"
                                value={filter[field.key]}
                                placeholder={field.placeholder}
                                list={options.length > 0 ? listId : undefined}
                                onChange={(e) => update(field.key, e.target.value)}
                            />
                            {options.length > 0 && (
                                <datalist id={listId}>
                                    {options.map((o) => (
                                        <option key={o} value={o} />
                                    ))}
                                </datalist>
                            )}
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
