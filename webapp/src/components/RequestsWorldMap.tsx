import { useRef, useMemo } from "react";
import { Map, Source, Layer } from "react-map-gl/maplibre";
import { RequestDetail } from "./RequestsDetailsTable";
import { getCountryCoordinates } from "../utils/countryCoordinates";
import maplibreStyleJson from "../assets/maplibre-style.json";

import type { MapRef, MapMouseEvent } from "react-map-gl/maplibre";
import type { GeoJSONSource } from "maplibre-gl";
import type { LayerProps } from "react-map-gl/maplibre";
import type { StyleSpecification } from "maplibre-gl";

interface RequestsWorldMapProps {
    requests: RequestDetail[];
}

// Cast the imported JSON to the correct type
const maplibreStyle = maplibreStyleJson as StyleSpecification;

// Layer definitions following the exact pattern from react-map-gl clusters example
export const clusterLayer: LayerProps = {
    id: 'clusters',
    type: 'circle',
    source: 'requests',
    filter: ['has', 'point_count'],
    paint: {
        'circle-color': ['step', ['get', 'point_count'], '#86efac', 5, '#4ade80', 10, '#22c55e', 20, '#16a34a', 30, '#15803d', 50, '#166534'],
        'circle-radius': ['step', ['get', 'point_count'], 12, 5, 16, 10, 20, 20, 24, 30, 28]
    }
};

export const clusterCountLayer: LayerProps = {
    id: 'cluster-count',
    type: 'symbol',
    source: 'requests',
    filter: ['has', 'point_count'],
    layout: {
        'text-field': '{point_count_abbreviated}',
        'text-size': 12
    },
    paint: {
        'text-color': [
            'step',
            ['get', 'point_count'],
            '#166534', // Dark green text for light green circles (1-4)
            5, '#166534', // Dark green text for medium-light circles (5-9)
            10, '#166534', // Dark green text for medium circles (10-19)
            20, '#ffffff', // White text for medium-dark circles (20-29)
            30, '#ffffff', // White text for dark circles (30-49)
            50, '#ffffff'  // White text for darkest circles (50+)
        ],
        'text-halo-color': [
            'step',
            ['get', 'point_count'],
            '#ffffff', // White halo for dark text on light circles
            5, '#ffffff',
            10, '#ffffff',
            20, '#000000', // Black halo for white text on dark circles
            30, '#000000',
            50, '#000000'
        ],
        'text-halo-width': 1.5,
        'text-halo-blur': 0.5
    }
};

export const unclusteredPointLayer: LayerProps = {
    id: 'unclustered-point',
    type: 'circle',
    source: 'requests',
    filter: ['!', ['has', 'point_count']],
    paint: {
        'circle-color': '#86efac',
        'circle-radius': 3,
        'circle-stroke-width': 1,
        'circle-stroke-color': '#fff'
    }
};

export function RequestsWorldMap({ requests }: RequestsWorldMapProps) {
    const mapRef = useRef<MapRef>(null);
    
    // Convert requests to GeoJSON points with coordinates from our lightweight lookup
    const geoJsonData = useMemo(() => {
        const features = requests.map((request, index) => {
            let longitude: number;
            let latitude: number;
            
            // Use actual coordinates from the request if available, otherwise fallback to country lookup
            if (request.latitude != null && request.longitude != null && 
                request.latitude !== 0 && request.longitude !== 0) {
                // Use the actual geolocation coordinates
                longitude = request.longitude;
                latitude = request.latitude;
            } else {
                // Fallback to country-based coordinates
                const country = request.country || "Unknown";
                const baseCoords = getCountryCoordinates(country);
                longitude = baseCoords[0];
                latitude = baseCoords[1];
            }
            
            // Ensure coordinates are within valid bounds
            longitude = Math.max(-180, Math.min(180, longitude));
            latitude = Math.max(-85, Math.min(85, latitude));
            
            return {
                type: "Feature" as const,
                properties: {
                    id: index,
                    country: request.country || "Unknown",
                    city: request.city,
                    path: request.path,
                    status_code: request.status_code,
                    response_time: request.response_time,
                    timestamp: request.timestamp,
                    username: request.username || "Anonymous",
                    hasActualCoordinates: request.latitude != null && request.longitude != null && 
                                         request.latitude !== 0 && request.longitude !== 0
                },
                geometry: {
                    type: "Point" as const,
                    coordinates: [longitude, latitude]
                }
            };
        });

        return {
            type: "FeatureCollection" as const,
            features
        };
    }, [requests]);

    // Count requests by country for the statistics section
    const countryData = useMemo(() => {
        return requests.reduce((acc, request) => {
            const country = request.country || "Unknown";
            acc[country] = (acc[country] || 0) + 1;
            return acc;
        }, {} as Record<string, number>);
    }, [requests]);

    // Click handler following the exact pattern from the example
    const onClick = async (event: MapMouseEvent) => {
        const feature = event.features?.[0];
        if (!feature) {
            return;
        }
        const clusterId = feature.properties?.cluster_id;

        // If it's a cluster, zoom in to expand it
        if (clusterId) {
            const geojsonSource = mapRef.current?.getSource('requests') as GeoJSONSource;
            if (geojsonSource && feature.geometry && 'coordinates' in feature.geometry) {
                try {
                    const zoom = await geojsonSource.getClusterExpansionZoom(clusterId);
                    mapRef.current?.easeTo({
                        center: feature.geometry.coordinates as [number, number],
                        zoom,
                        duration: 500
                    });
                } catch (error) {
                    console.warn('Could not get cluster expansion zoom:', error);
                }
            }
        }
    };

    if (requests.length === 0) {
        return (
            <div className="w-full bg-white border border-gray-200 rounded-lg p-4">
                <h3 className="text-lg font-semibold mb-4">Request Clusters</h3>
                <div className="text-center py-8 text-gray-500">
                    No request data available for the selected period
                </div>
            </div>
        );
    }

    return (
        <div className="w-full bg-white border border-gray-200 rounded-lg p-4">
            <h3 className="text-lg font-semibold mb-4">Request Clusters</h3>
            
            {/* Legend */}
            <div className="mb-4 flex flex-wrap items-center gap-4 text-sm text-gray-600">
                <span>Cluster Size:</span>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#86efac] rounded-full"></div>
                    <span>1-4</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#4ade80] rounded-full"></div>
                    <span>5-9</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#22c55e] rounded-full"></div>
                    <span>10-19</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#16a34a] rounded-full"></div>
                    <span>20-29</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#15803d] rounded-full"></div>
                    <span>30-49</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#166534] rounded-full"></div>
                    <span>50+</span>
                </div>
                <span className="ml-4">Total: {requests.length} requests</span>
            </div>

            {/* Map following the exact pattern from the example */}
            <div className="w-full h-[500px] overflow-hidden rounded-lg">
                <Map
                    initialViewState={{
                        latitude: 20,
                        longitude: 0,
                        zoom: 1.0
                    }}
                    mapStyle={maplibreStyle}
                    interactiveLayerIds={[clusterLayer.id!]}
                    onClick={onClick}
                    ref={mapRef}
                >
                    <Source
                        id="requests"
                        type="geojson"
                        data={geoJsonData}
                        cluster={true}
                        clusterMaxZoom={14}
                        clusterRadius={50}
                    >
                        <Layer {...clusterLayer} />
                        <Layer {...clusterCountLayer} />
                        <Layer {...unclusteredPointLayer} />
                    </Source>
                </Map>
            </div>

            {/* Country statistics */}
            {Object.keys(countryData).length > 0 && (
                <div className="mt-4">
                    <h4 className="text-md font-medium mb-2">Top Countries</h4>
                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2 text-sm">
                        {Object.entries(countryData)
                            .sort(([, a], [, b]) => b - a)
                            .slice(0, 8)
                            .map(([country, count]) => (
                                <div key={country} className="flex justify-between bg-gray-50 px-2 py-1 rounded">
                                    <span className="truncate">{country}</span>
                                    <span className="font-medium">{count}</span>
                                </div>
                            ))
                        }
                    </div>
                    
                    {/* Coordinate accuracy note */}
                    <div className="mt-3 text-xs text-gray-500">
                        {(() => {
                            const actualCoords = requests.filter(r => r.latitude != null && r.longitude != null && 
                                                                   r.latitude !== 0 && r.longitude !== 0).length;
                            const fallbackCoords = requests.length - actualCoords;
                            
                            if (actualCoords > 0 && fallbackCoords > 0) {
                                return `📍 ${actualCoords} precise locations, ${fallbackCoords} country-based approximations`;
                            } else if (actualCoords > 0) {
                                return `📍 All locations show precise coordinates`;
                            } else {
                                return `📍 All locations are country-based approximations`;
                            }
                        })()}
                    </div>
                </div>
            )}
        </div>
    );
}
