import { StatisticsDateRange } from './StatisticsDateRange';
import { RequestTypeFilter } from './RequestTypeFilter';
import { trafficDimensionFields } from '../lib/trafficFilter';
import type { TrafficFilterController } from '../lib/useTrafficFilter';

export function TrafficFilterBar({ controller }: { controller: TrafficFilterController }) {
    const { filter, period, setPeriod, setDateRange, update } = controller;

    return (
        <div className="space-y-3">
            <div className="flex flex-wrap items-end gap-4">
                <StatisticsDateRange
                    dateRange={{ startDate: filter.startDate, endDate: filter.endDate }}
                    setDateRange={setDateRange}
                    selectedPeriod={period}
                    setSelectedPeriod={setPeriod}
                />
                <RequestTypeFilter value={filter.requestType} onChange={(v) => update('requestType', v)} />
            </div>
            <div className="flex flex-wrap items-end gap-4">
                {trafficDimensionFields.map((field) => (
                    <div key={field.key}>
                        <label className="block text-sm font-medium text-muted-fg" htmlFor={`traffic-filter-${field.key}`}>
                            {field.label}
                        </label>
                        <input
                            id={`traffic-filter-${field.key}`}
                            className="tg-input py-1.5"
                            value={filter[field.key]}
                            placeholder={field.placeholder}
                            onChange={(e) => update(field.key, e.target.value)}
                        />
                    </div>
                ))}
            </div>
        </div>
    );
}
