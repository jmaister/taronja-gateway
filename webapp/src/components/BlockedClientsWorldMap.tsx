import { useRef, useMemo } from "react";
import { Map, Source, Layer } from "react-map-gl/maplibre";
import { getCountryCoordinates } from "../utils/countryCoordinates";
import maplibreStyleJson from "../assets/maplibre-style.json";

import type { MapRef, MapMouseEvent } from "react-map-gl/maplibre";
import type { GeoJSONSource } from "maplibre-gl";
import type { LayerProps } from "react-map-gl/maplibre";
import type { StyleSpecification } from "maplibre-gl";
import { BlockedClient } from "@/apiclient";

interface BlockedClientsWorldMapProps {
    blockedClients: BlockedClient[];
}

// See RequestsWorldMap.tsx's identical comment: JSON module imports infer
// loose types, so this indirection through `unknown` is needed to satisfy
// StyleSpecification.
const maplibreStyle = maplibreStyleJson as unknown as StyleSpecification;

// Same clustering approach as RequestsWorldMap, but in a red/orange ramp —
// this map is about attackers, not general traffic, so it's deliberately
// visually distinct from the green "Request Clusters" map.
export const blockedClusterLayer: LayerProps = {
    id: 'blocked-clusters',
    type: 'circle',
    source: 'blocked-clients',
    filter: ['has', 'point_count'],
    paint: {
        'circle-color': ['step', ['get', 'point_count'], '#fca5a5', 5, '#f87171', 10, '#ef4444', 20, '#dc2626', 30, '#b91c1c', 50, '#7f1d1d'],
        'circle-radius': ['step', ['get', 'point_count'], 12, 5, 16, 10, 20, 20, 24, 30, 28]
    }
};

export const blockedClusterCountLayer: LayerProps = {
    id: 'blocked-cluster-count',
    type: 'symbol',
    source: 'blocked-clients',
    filter: ['has', 'point_count'],
    layout: {
        'text-field': '{point_count_abbreviated}',
        'text-size': 12
    },
    paint: {
        'text-color': '#ffffff',
        'text-halo-color': '#000000',
        'text-halo-width': 1.5,
        'text-halo-blur': 0.5
    }
};

export const blockedUnclusteredPointLayer: LayerProps = {
    id: 'blocked-unclustered-point',
    type: 'circle',
    source: 'blocked-clients',
    filter: ['!', ['has', 'point_count']],
    paint: {
        'circle-color': '#ef4444',
        'circle-radius': 4,
        'circle-stroke-width': 1,
        'circle-stroke-color': '#fff'
    }
};

export function BlockedClientsWorldMap({ blockedClients }: BlockedClientsWorldMapProps) {
    const mapRef = useRef<MapRef>(null);

    // Convert blocked clients to GeoJSON points, same coordinate strategy as
    // RequestsWorldMap: prefer the recorded GPS coordinates, fall back to a
    // country-centroid lookup when they're missing (e.g. localhost blocks in
    // dev, or a geolocation lookup that failed).
    const geoJsonData = useMemo(() => {
        const features = blockedClients.map((bc, index) => {
            let longitude: number;
            let latitude: number;

            if (bc.latitude != null && bc.longitude != null && bc.latitude !== 0 && bc.longitude !== 0) {
                longitude = bc.longitude;
                latitude = bc.latitude;
            } else {
                const country = bc.country || "Unknown";
                const baseCoords = getCountryCoordinates(country);
                longitude = baseCoords[0];
                latitude = baseCoords[1];
            }

            longitude = Math.max(-180, Math.min(180, longitude));
            latitude = Math.max(-85, Math.min(85, latitude));

            return {
                type: "Feature" as const,
                properties: {
                    id: index,
                    ipAddress: bc.ipAddress,
                    reason: bc.reason,
                    path: bc.path,
                    triggerCount: bc.triggerCount,
                    blockedAt: bc.blockedAt,
                    blockedUntil: bc.blockedUntil,
                    country: bc.country || "Unknown",
                    city: bc.city,
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
    }, [blockedClients]);

    // Count blocks by country for the "Top Countries" section below the map.
    const countryData = useMemo(() => {
        return blockedClients.reduce((acc, bc) => {
            const country = bc.country || "Unknown";
            acc[country] = (acc[country] || 0) + 1;
            return acc;
        }, {} as Record<string, number>);
    }, [blockedClients]);

    const onClick = async (event: MapMouseEvent) => {
        const feature = event.features?.[0];
        if (!feature) {
            return;
        }
        const clusterId = feature.properties?.cluster_id;

        if (clusterId) {
            const geojsonSource = mapRef.current?.getSource('blocked-clients') as GeoJSONSource;
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

    if (blockedClients.length === 0) {
        return (
            <div className="w-full bg-white border border-gray-200 rounded-lg p-4">
                <h3 className="text-lg font-semibold mb-4">Attacker Map</h3>
                <div className="text-center py-8 text-gray-500">
                    No blocked clients recorded yet.
                </div>
            </div>
        );
    }

    return (
        <div className="w-full bg-white border border-gray-200 rounded-lg p-4">
            <h3 className="text-lg font-semibold mb-4">Attacker Map</h3>

            {/* Legend */}
            <div className="mb-4 flex flex-wrap items-center gap-4 text-sm text-gray-600">
                <span>Block Count:</span>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#fca5a5] rounded-full"></div>
                    <span>1-4</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#f87171] rounded-full"></div>
                    <span>5-9</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#ef4444] rounded-full"></div>
                    <span>10-19</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#dc2626] rounded-full"></div>
                    <span>20-29</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#b91c1c] rounded-full"></div>
                    <span>30-49</span>
                </div>
                <div className="flex items-center gap-2">
                    <div className="w-4 h-4 bg-[#7f1d1d] rounded-full"></div>
                    <span>50+</span>
                </div>
                <span className="ml-4">Total: {blockedClients.length} blocks</span>
            </div>

            <div className="w-full h-[500px] overflow-hidden rounded-lg">
                <Map
                    initialViewState={{
                        latitude: 20,
                        longitude: 0,
                        zoom: 1.0
                    }}
                    mapStyle={maplibreStyle}
                    interactiveLayerIds={[blockedClusterLayer.id!]}
                    onClick={onClick}
                    ref={mapRef}
                >
                    <Source
                        id="blocked-clients"
                        type="geojson"
                        data={geoJsonData}
                        cluster={true}
                        clusterMaxZoom={14}
                        clusterRadius={50}
                    >
                        <Layer {...blockedClusterLayer} />
                        <Layer {...blockedClusterCountLayer} />
                        <Layer {...blockedUnclusteredPointLayer} />
                    </Source>
                </Map>
            </div>

            {/* Country statistics */}
            {Object.keys(countryData).length > 0 && (
                <div className="mt-4">
                    <h4 className="text-md font-medium mb-2">Top Attacker Countries</h4>
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
                </div>
            )}
        </div>
    );
}
