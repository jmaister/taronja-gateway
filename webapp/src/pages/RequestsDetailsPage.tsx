import { useState } from "react";
import { RequestsDetailsTable } from "../components/RequestsDetailsTable";
import { StatisticsDateRange, timePeriods, DateRange } from "../components/StatisticsDateRange";
import { LazyRequestsWorldMap } from "../components/LazyRequestsWorldMap";
import { useRequestDetails } from "../services/services";
import { Button } from "../components/ui/Button";
import { Card, CardContent } from "../components/ui/Card";
import { PageHeader } from "../components/ui/PageHeader";

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
        <div className="mx-auto max-w-7xl space-y-6">
            <PageHeader
                title="Request Details"
                description="Explore individual requests and geolocation clusters"
                actions={
                    <>
                        <Button onClick={() => refetch()} disabled={isLoading}>
                            <span className={`${isLoading ? 'animate-spin' : ''}`}>
                                {isLoading ? '⟳' : '🔄'}
                            </span>
                            Refresh
                        </Button>
                        <StatisticsDateRange
                            dateRange={dateRange}
                            setDateRange={setDateRange}
                            selectedPeriod={selectedPeriod}
                            setSelectedPeriod={setSelectedPeriod}
                        />
                    </>
                }
            />
            
            {error && (
                <Card className="border-danger/30 bg-danger/5">
                    <CardContent className="py-4">
                        <div className="flex items-center">
                            <div className="mr-3 text-xl text-danger">⚠️</div>
                            <div>
                                <h3 className="font-medium text-danger">Error Loading Request Details</h3>
                                <p className="mt-1 text-sm text-danger/80">
                                    {error instanceof Error ? error.message : 'Unknown error'}
                                </p>
                            </div>
                            <div className="ml-auto">
                                <Button variant="danger" size="sm" onClick={() => refetch()}>
                                    Retry
                                </Button>
                            </div>
                        </div>
                    </CardContent>
                </Card>
            )}

            {isLoading ? (
                <div className="py-12 text-center text-sm text-muted-fg">Loading…</div>
            ) : (
                <div className="w-full space-y-6">
                    <LazyRequestsWorldMap requests={requests} />
                    <RequestsDetailsTable requests={requests} />
                </div>
            )}
        </div>
    );
}
