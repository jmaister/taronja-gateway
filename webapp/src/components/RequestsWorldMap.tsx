import { ClusteredWorldMap } from "./ClusteredWorldMap";
import { RequestDetail } from "@/apiclient";

interface RequestsWorldMapProps {
    requests: RequestDetail[];
}

const hasActualCoordinates = (request: RequestDetail) =>
    request.latitude != null && request.longitude != null &&
    request.latitude !== 0 && request.longitude !== 0;

const legend = [
    { color: '#86efac', label: '1-4' },
    { color: '#4ade80', label: '5-9' },
    { color: '#22c55e', label: '10-19' },
    { color: '#16a34a', label: '20-29' },
    { color: '#15803d', label: '30-49' },
    { color: '#166534', label: '50+' },
];

const colors = {
    base: '#86efac',
    stops: [
        { breakpoint: 5, color: '#4ade80' },
        { breakpoint: 10, color: '#22c55e' },
        { breakpoint: 20, color: '#16a34a' },
        { breakpoint: 30, color: '#15803d' },
        { breakpoint: 50, color: '#166534' },
    ],
    point: '#86efac',
};

export function RequestsWorldMap({ requests }: RequestsWorldMapProps) {
    return (
        <ClusteredWorldMap
            items={requests}
            sourceId="requests"
            title="Request Clusters"
            emptyMessage="No request data available for the selected period"
            legendLabel="Cluster Size:"
            legend={legend}
            countNoun="requests"
            countryStatsTitle="Top Countries"
            colors={colors}
            getCoordinates={(request) => ({
                longitude: request.longitude ?? 0,
                latitude: request.latitude ?? 0,
                hasActualCoordinates: hasActualCoordinates(request),
            })}
            getCountry={(request) => request.country || "Unknown"}
            getProperties={(request) => ({
                country: request.country || "Unknown",
                city: request.city,
                path: request.path,
                status_code: request.status_code,
                response_time: request.response_time,
                timestamp: request.timestamp,
                username: request.username || "Anonymous",
            })}
        />
    );
}
