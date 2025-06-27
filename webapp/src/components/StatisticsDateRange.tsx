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
            const start = todayDate;
            start.setDate(todayDate.getDate() - todayDate.getDay() + 1);
            const end = new Date(start);
            end.setDate(start.getDate() + 6);
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
        <div className="flex gap-4 items-end mb-4">
            <div>
                <label className="block text-sm font-medium">Period</label>
                <select
                    className="border rounded px-2 py-1"
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
                <label className="block text-sm font-medium">Start Date</label>
                <input
                    type="date"
                    className="border rounded px-2 py-1"
                    value={dateRange.startDate}
                    onChange={(e) => handleDateChange("startDate", e.target.value)}
                    disabled={selectedPeriod !== "other"}
                />
            </div>
            <div>
                <label className="block text-sm font-medium">End Date</label>
                <input
                    type="date"
                    className="border rounded px-2 py-1"
                    value={dateRange.endDate}
                    onChange={(e) => handleDateChange("endDate", e.target.value)}
                    disabled={selectedPeriod !== "other"}
                />
            </div>
        </div>
    );
}
