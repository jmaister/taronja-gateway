import { ClusteredWorldMap } from "./ClusteredWorldMap";
import { BlockedClient } from "@/apiclient";

interface BlockedClientsWorldMapProps {
    blockedClients: BlockedClient[];
}

const hasActualCoordinates = (bc: BlockedClient) =>
    bc.latitude != null && bc.longitude != null && bc.latitude !== 0 && bc.longitude !== 0;

// Red/orange ramp — this map is about attackers, not general traffic, so
// it's deliberately visually distinct from the green "Request Clusters" map.
const legend = [
    { color: '#fca5a5', label: '1-4' },
    { color: '#f87171', label: '5-9' },
    { color: '#ef4444', label: '10-19' },
    { color: '#dc2626', label: '20-29' },
    { color: '#b91c1c', label: '30-49' },
    { color: '#7f1d1d', label: '50+' },
];

const colors = {
    base: '#fca5a5',
    stops: [
        { breakpoint: 5, color: '#f87171' },
        { breakpoint: 10, color: '#ef4444' },
        { breakpoint: 20, color: '#dc2626' },
        { breakpoint: 30, color: '#b91c1c' },
        { breakpoint: 50, color: '#7f1d1d' },
    ],
    point: '#ef4444',
};

export function BlockedClientsWorldMap({ blockedClients }: BlockedClientsWorldMapProps) {
    return (
        <ClusteredWorldMap
            items={blockedClients}
            sourceId="blocked-clients"
            title="Attacker Map"
            emptyMessage="No blocked clients recorded yet."
            legendLabel="Block Count:"
            legend={legend}
            countNoun="blocks"
            countryStatsTitle="Top Attacker Countries"
            colors={colors}
            getCoordinates={(bc) => ({
                longitude: bc.longitude ?? 0,
                latitude: bc.latitude ?? 0,
                hasActualCoordinates: hasActualCoordinates(bc),
            })}
            getCountry={(bc) => bc.country || "Unknown"}
            getProperties={(bc) => ({
                ipAddress: bc.ipAddress,
                reason: bc.reason,
                path: bc.path,
                triggerCount: bc.triggerCount,
                blockedAt: bc.blockedAt,
                blockedUntil: bc.blockedUntil,
                country: bc.country || "Unknown",
                city: bc.city,
            })}
        />
    );
}
