import { RequestDetail } from "@/apiclient";
import { lazy, Suspense } from "react";

// Lazy load the heavy map component
const RequestsWorldMapInternal = lazy(() => 
    import("./RequestsWorldMap").then(module => ({ default: module.RequestsWorldMap }))
);

interface LazyRequestsWorldMapProps {
    requests: RequestDetail[];
}

function MapLoadingFallback() {
    return (
        <div className="w-full bg-white border border-gray-200 rounded-lg p-4">
            <h3 className="text-lg font-semibold mb-4">Request Clusters</h3>
            <div className="w-full h-[500px] flex items-center justify-center bg-gray-50 rounded-lg">
                <div className="text-center">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto mb-2"></div>
                    <p className="text-gray-600">Loading map...</p>
                </div>
            </div>
        </div>
    );
}

export function LazyRequestsWorldMap({ requests }: LazyRequestsWorldMapProps) {
    return (
        <Suspense fallback={<MapLoadingFallback />}>
            <RequestsWorldMapInternal requests={requests} />
        </Suspense>
    );
}
