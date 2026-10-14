import { RequestsDetailsTable } from "../components/RequestsDetailsTable";
import { TrafficFilterBar } from "../components/TrafficFilterBar";
import { QueryState } from "../components/QueryState";
import { LazyRequestsWorldMap } from "../components/LazyRequestsWorldMap";
import { useRequestDetails } from "../services/statistics";
import { useTrafficFilter } from "../lib/useTrafficFilter";
import { toggleValue } from "../lib/toggleValue";
import { Button } from "../components/ui/Button";
import { PageHeader } from "../components/ui/PageHeader";

export function RequestsDetailsPage() {
    const controller = useTrafficFilter();
    const { data, isLoading, error, refetch } = useRequestDetails(controller.filter);

    const requests = data?.requests || [];
    const country = controller.filter.country;
    const selectCountry = (value: string) => controller.update('country', toggleValue(country, value));

    return (
        <div className="mx-auto max-w-7xl space-y-6">
            <PageHeader
                title="Request Details"
                description="Explore individual requests and geolocation clusters"
                actions={
                    <Button onClick={() => refetch()} disabled={isLoading}>
                        <span className={`${isLoading ? 'animate-spin' : ''}`}>
                            {isLoading ? '⟳' : '🔄'}
                        </span>
                        Refresh
                    </Button>
                }
            />
            <TrafficFilterBar controller={controller} />

            <QueryState isLoading={isLoading} error={error} onRetry={() => refetch()} errorTitle="Error Loading Request Details">
                <div className="w-full space-y-6">
                    <LazyRequestsWorldMap requests={requests} activeCountry={country} onCountrySelect={selectCountry} />
                    <RequestsDetailsTable requests={requests} activeCountry={country} onCountrySelect={selectCountry} />
                </div>
            </QueryState>
        </div>
    );
}
