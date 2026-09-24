import { format } from "date-fns";

export type DateRange = {
    startDate: string;
    endDate: string;
};

export type TimePeriod = {
    label: string;
    value: string;
    getDateRange: () => { startDate: string; endDate: string };
};

const todayDate = new Date();
todayDate.setHours(0, 0, 0, 0);

// startOfWeek returns the Monday on or before date, as a new Date — it
// never mutates its argument, unlike the old "This Week" calculation this
// replaces, which wrote through to the module-level todayDate singleton
// (start.setDate(...) on `const start = todayDate`) and so permanently
// shifted every period's idea of "today" after "This Week" was picked
// once. getDay() treats Sunday as day 0, so it's the one case rolled back
// 6 days rather than forward.
function startOfWeek(date: Date): Date {
    const start = new Date(date);
    const day = start.getDay();
    start.setDate(start.getDate() + (day === 0 ? -6 : 1 - day));
    return start;
}

export const timePeriods: TimePeriod[] = [
    {
        label: "Today",
        value: "today",
        getDateRange: () => {
            const dateStr = format(todayDate, "yyyy-MM-dd");
            return { startDate: dateStr, endDate: dateStr };
        },
    },
    {
        label: "Yesterday",
        value: "yesterday",
        getDateRange: () => {
            const yesterday = new Date(todayDate);
            yesterday.setDate(todayDate.getDate() - 1);
            const dateStr = format(yesterday, "yyyy-MM-dd");
            return { startDate: dateStr, endDate: dateStr };
        },
    },
    {
        label: "This Week",
        value: "thisWeek",
        getDateRange: () => {
            const start = startOfWeek(todayDate);
            const end = new Date(start);
            end.setDate(start.getDate() + 6);
            return {
                startDate: format(start, "yyyy-MM-dd"),
                endDate: format(end, "yyyy-MM-dd"),
            };
        },
    },
    {
        label: "Last Week",
        value: "lastWeek",
        getDateRange: () => {
            const thisWeekStart = startOfWeek(todayDate);
            const start = new Date(thisWeekStart);
            start.setDate(thisWeekStart.getDate() - 7);
            const end = new Date(thisWeekStart);
            end.setDate(thisWeekStart.getDate() - 1);
            return {
                startDate: format(start, "yyyy-MM-dd"),
                endDate: format(end, "yyyy-MM-dd"),
            };
        },
    },
    {
        label: "This Month",
        value: "thisMonth",
        getDateRange: () => {
            const start = new Date(todayDate.getFullYear(), todayDate.getMonth(), 1);
            const end = new Date(todayDate.getFullYear(), todayDate.getMonth() + 1, 0);
            return {
                startDate: format(start, "yyyy-MM-dd"),
                endDate: format(end, "yyyy-MM-dd"),
            };
        },
    },
    {
        label: "Last Month",
        value: "lastMonth",
        getDateRange: () => {
            const start = new Date(todayDate.getFullYear(), todayDate.getMonth() - 1, 1);
            const end = new Date(todayDate.getFullYear(), todayDate.getMonth(), 0);
            return {
                startDate: format(start, "yyyy-MM-dd"),
                endDate: format(end, "yyyy-MM-dd"),
            };
        },
    },
    {
        label: "Other...",
        value: "other",
        getDateRange: () => {
            const dateStr = format(todayDate, "yyyy-MM-dd");
            return { startDate: dateStr, endDate: dateStr };
        },
    },
];

export function StatisticsDateRange({
    dateRange,
    setDateRange,
    selectedPeriod,
    setSelectedPeriod,
}: {
    dateRange: DateRange;
    setDateRange: (range: DateRange) => void;
    selectedPeriod: string;
    setSelectedPeriod: (period: string) => void;
}) {
    function handlePeriodChange(period: string) {
        setSelectedPeriod(period);
        if (period !== "other") {
            const found = timePeriods.find((p) => p.value === period);
            if (found) setDateRange(found.getDateRange());
        }
    }
    function handleDateChange(field: keyof DateRange, value: string) {
        setDateRange({ ...dateRange, [field]: value });
    }
    return (
        <div className="flex items-end gap-4">
            <div>
                <label className="block text-sm font-medium text-muted-fg">Period</label>
                <select
                    className="tg-input py-1.5"
                    value={selectedPeriod}
                    onChange={(e) => handlePeriodChange(e.target.value)}
                >
                    {timePeriods.map((p) => (
                        <option key={p.value} value={p.value}>
                            {p.label}
                        </option>
                    ))}
                </select>
            </div>
            <div>
                <label className="block text-sm font-medium text-muted-fg">Start Date</label>
                <input
                    type="date"
                    className="tg-input py-1.5"
                    value={dateRange.startDate}
                    onChange={(e) => handleDateChange("startDate", e.target.value)}
                    disabled={selectedPeriod !== "other"}
                />
            </div>
            <div>
                <label className="block text-sm font-medium text-muted-fg">End Date</label>
                <input
                    type="date"
                    className="tg-input py-1.5"
                    value={dateRange.endDate}
                    onChange={(e) => handleDateChange("endDate", e.target.value)}
                    disabled={selectedPeriod !== "other"}
                />
            </div>
        </div>
    );
}
