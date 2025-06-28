import { useEffect, useState } from "react";
import { RequestsDetailsTable, RequestDetail } from "../components/RequestsDetailsTable";
import { StatisticsDateRange, timePeriods, DateRange } from "../components/StatisticsDateRange";
import { LazyRequestsWorldMap } from "../components/LazyRequestsWorldMap";

export default function RequestsDetailsPage() {
    const [requests, setRequests] = useState<RequestDetail[]>([]);
    const [selectedPeriod, setSelectedPeriod] = useState<string>("today");
    const [dateRange, setDateRange] = useState<DateRange>(() => timePeriods[0].getDateRange());
    const [loading, setLoading] = useState(false);

    useEffect(() => {
        fetchRequests();
        // eslint-disable-next-line
    }, [dateRange]);

    function fetchRequests() {
        setLoading(true);
        let url = "/_/api/statistics/requests/details";
        const params = [];
        if (dateRange.startDate) {
            params.push(`start_date=${dateRange.startDate}T00:00:00Z`);
        }
        if (dateRange.endDate) {
            params.push(`end_date=${dateRange.endDate}T23:59:59Z`);
        }
        if (params.length) {
            url += `?${params.join("&")}`;
        }
        fetch(url)
            .then((res) => res.json())
            .then((data) => setRequests(data.requests || []))
            .finally(() => setLoading(false));
    }

    return (
        <div className="p-6 w-full">
            <h1 className="text-2xl font-bold mb-4">Request Details</h1>
            <StatisticsDateRange
                dateRange={dateRange}
                setDateRange={setDateRange}
                selectedPeriod={selectedPeriod}
                setSelectedPeriod={setSelectedPeriod}
            />
            {loading ? (
                <div className="text-center py-8 text-gray-500">Loading...</div>
            ) : (
                <div className="w-full space-y-6">
                    <LazyRequestsWorldMap requests={requests} />
                    <RequestsDetailsTable requests={requests} />
                </div>
            )}
        </div>
    );
}
