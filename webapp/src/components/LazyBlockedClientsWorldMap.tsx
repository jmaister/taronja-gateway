import { BlockedClient } from "@/apiclient";
import { lazy, Suspense } from "react";

// Lazy load the heavy map component — see LazyRequestsWorldMap.tsx, same
// rationale (maplibre-gl is a large dependency not worth it on first paint).
const BlockedClientsWorldMapInternal = lazy(() =>
    import("./BlockedClientsWorldMap").then(module => ({ default: module.BlockedClientsWorldMap }))
);

interface LazyBlockedClientsWorldMapProps {
    blockedClients: BlockedClient[];
}

function MapLoadingFallback() {
    return (
        <div className="w-full bg-white border border-gray-200 rounded-lg p-4">
            <h3 className="text-lg font-semibold mb-4">Attacker Map</h3>
            <div className="w-full h-[500px] flex items-center justify-center bg-gray-50 rounded-lg">
                <div className="text-center">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto mb-2"></div>
                    <p className="text-gray-600">Loading map...</p>
                </div>
            </div>
        </div>
    );
}

export function LazyBlockedClientsWorldMap({ blockedClients }: LazyBlockedClientsWorldMapProps) {
    return (
        <Suspense fallback={<MapLoadingFallback />}>
            <BlockedClientsWorldMapInternal blockedClients={blockedClients} />
        </Suspense>
    );
}
