import { useState } from "react";
import { RequestsDetailsTable } from "../components/RequestsDetailsTable";
import { StatisticsDateRange, timePeriods, DateRange } from "../components/StatisticsDateRange";
import { LazyRequestsWorldMap } from "../components/LazyRequestsWorldMap";
import { useRequestDetails } from "../services/services";

export function RequestsDetailsPage() {
    const [selectedPeriod, setSelectedPeriod] = useState<string>("today");
    const [dateRange, setDateRange] = useState<DateRange>(() => timePeriods[0].getDateRange());

    const startDateStr = `${dateRange.startDate}T00:00:00Z`;
    const endDateStr = `${dateRange.endDate}T23:59:59Z`;

    const { 
        data, 
        isLoading, 
        error, 
        refetch 
    } = useRequestDetails(startDateStr, endDateStr);

    // Ensure we always have an array, even if the API returns unexpected data
    const requests = data?.requests || [];

    return (
        <div className="p-6 w-full">
            <div className="flex justify-between items-center mb-4">
                <h1 className="text-2xl font-bold">Request Details</h1>
                <div className="flex items-center space-x-4">
                    <button
                        onClick={() => refetch()}
                        disabled={isLoading}
                        className="flex items-center px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                    >
                        <span className={`mr-2 ${isLoading ? 'animate-spin' : ''}`}>
                            {isLoading ? '⟳' : '🔄'}
                        </span>
                        Refresh
                    </button>
                    <StatisticsDateRange
                        dateRange={dateRange}
                        setDateRange={setDateRange}
                        selectedPeriod={selectedPeriod}
                        setSelectedPeriod={setSelectedPeriod}
                    />
                </div>
            </div>
            
            {error && (
                <div className="bg-red-50 border border-red-200 rounded-lg p-4 mb-6">
                    <div className="flex items-center">
                        <div className="text-red-500 text-xl mr-3">⚠️</div>
                        <div>
                            <h3 className="text-red-800 font-medium">Error Loading Request Details</h3>
                            <p className="text-red-600 text-sm mt-1">
                                {error instanceof Error ? error.message : 'Unknown error'}
                            </p>
                        </div>
                        <button
                            onClick={() => refetch()}
                            className="ml-auto px-3 py-1 bg-red-500 text-white text-sm rounded hover:bg-red-600 transition-colors"
                        >
                            Retry
                        </button>
                    </div>
                </div>
            )}

            {isLoading ? (
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
