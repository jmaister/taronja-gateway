import { useRef, useMemo } from "react";
import { Map, Source, Layer } from "react-map-gl/maplibre";
import { getCountryCoordinates } from "../utils/countryCoordinates";
import { Card } from "./ui/Card";
import maplibreStyleJson from "../assets/maplibre-style.json";

import type { MapRef, MapMouseEvent } from "react-map-gl/maplibre";
import type { GeoJSONSource } from "maplibre-gl";
import type { LayerProps } from "react-map-gl/maplibre";
import type { StyleSpecification } from "maplibre-gl";

// Cast the imported JSON to the correct type. JSON module imports infer
// loose types (e.g. array literals like `center` widen to `number[]`
// instead of the `[number, number]` tuple StyleSpecification expects), so a
// direct assertion doesn't type-check even though the JSON is a valid style
// at runtime. Going through `unknown` first is the standard, TS-suggested
// way to assert a type the compiler can't otherwise verify overlaps.
const maplibreStyle = maplibreStyleJson as unknown as StyleSpecification;

export interface ClusterColorStop {
    /** point_count breakpoint this color starts applying at. */
    breakpoint: number;
    color: string;
}

export interface ClusterColors {
    /** Circle color for the smallest bucket, below the first stop's breakpoint. */
    base: string;
    /** Ascending breakpoints — e.g. [{breakpoint: 5, color: '#f87171'}, ...]. */
    stops: ClusterColorStop[];
    /** Color for an individual, unclustered point. */
    point: string;
}

/** One legend swatch — a color paired with the count-range label under it. */
export interface LegendEntry {
    color: string;
    label: string;
}

export interface ClusteredWorldMapProps<T> {
    items: T[];
    /**
     * Unique per rendered instance — becomes the maplibre source id and the
     * prefix for its layer ids, so two of these on the same page (not
     * currently the case, but cheap to keep safe) never collide.
     */
    sourceId: string;
    title: string;
    emptyMessage: string;
    legendLabel: string;
    legend: LegendEntry[];
    /** e.g. "requests" / "blocks" — used in "Total: {n} {countNoun}". */
    countNoun: string;
    countryStatsTitle: string;
    colors: ClusterColors;
    /** GPS coordinates for one item, plus whether they're real (vs. a country-centroid fallback). */
    getCoordinates: (item: T) => { longitude: number; latitude: number; hasActualCoordinates: boolean };
    getCountry: (item: T) => string;
    /**
     * Arbitrary per-item fields to attach to its GeoJSON feature, for a
     * future popup/tooltip to read off `feature.properties` — the click
     * handler itself only ever reads maplibre/supercluster's own injected
     * `cluster_id`, so this is forward-compatibility, not something this
     * component reads itself.
     */
    getProperties?: (item: T, index: number) => Record<string, unknown>;
}

export function ClusteredWorldMap<T>({
    items,
    sourceId,
    title,
    emptyMessage,
    legendLabel,
    legend,
    countNoun,
    countryStatsTitle,
    colors,
    getCoordinates,
    getCountry,
    getProperties,
}: ClusteredWorldMapProps<T>) {
    const mapRef = useRef<MapRef>(null);

    const clusterLayer: LayerProps = useMemo(() => ({
        id: `${sourceId}-clusters`,
        type: 'circle',
        source: sourceId,
        filter: ['has', 'point_count'],
        paint: {
            'circle-color': [
                'step', ['get', 'point_count'], colors.base,
                ...colors.stops.flatMap(({ breakpoint, color }) => [breakpoint, color]),
            ],
            'circle-radius': ['step', ['get', 'point_count'], 12, 5, 16, 10, 20, 20, 24, 30, 28],
        },
    }), [sourceId, colors]);

    const clusterCountLayer: LayerProps = useMemo(() => ({
        id: `${sourceId}-cluster-count`,
        type: 'symbol',
        source: sourceId,
        filter: ['has', 'point_count'],
        layout: {
            'text-field': '{point_count_abbreviated}',
            'text-size': 12,
        },
        paint: {
            // White-on-black-halo reads clearly against every color in
            // either palette this component is used with, so there's no
            // need to vary it by cluster size the way a single fixed
            // palette's legibility might otherwise require.
            'text-color': '#ffffff',
            'text-halo-color': '#000000',
            'text-halo-width': 1.5,
            'text-halo-blur': 0.5,
        },
    }), [sourceId]);

    const unclusteredPointLayer: LayerProps = useMemo(() => ({
        id: `${sourceId}-unclustered-point`,
        type: 'circle',
        source: sourceId,
        filter: ['!', ['has', 'point_count']],
        paint: {
            'circle-color': colors.point,
            'circle-radius': 3,
            'circle-stroke-width': 1,
            'circle-stroke-color': '#fff',
        },
    }), [sourceId, colors]);

    // Convert items to GeoJSON points: prefer each item's own GPS
    // coordinates, falling back to a country-centroid lookup when they're
    // missing (e.g. localhost traffic in dev, or a failed geolocation
    // lookup).
    const geoJsonData = useMemo(() => {
        const features = items.map((item, index) => {
            const coords = getCoordinates(item);
            const longitude = Math.max(-180, Math.min(180, coords.hasActualCoordinates
                ? coords.longitude
                : getCountryCoordinates(getCountry(item))[0]));
            const latitude = Math.max(-85, Math.min(85, coords.hasActualCoordinates
                ? coords.latitude
                : getCountryCoordinates(getCountry(item))[1]));

            return {
                type: "Feature" as const,
                properties: {
                    id: index,
                    hasActualCoordinates: coords.hasActualCoordinates,
                    ...getProperties?.(item, index),
                },
                geometry: {
                    type: "Point" as const,
                    coordinates: [longitude, latitude],
                },
            };
        });

        return {
            type: "FeatureCollection" as const,
            features,
        };
    }, [items, getCoordinates, getCountry, getProperties]);

    // Count items by country for the "Top Countries"-style section below the map.
    const countryData = useMemo(() => {
        return items.reduce((acc, item) => {
            const country = getCountry(item);
            acc[country] = (acc[country] || 0) + 1;
            return acc;
        }, {} as Record<string, number>);
    }, [items, getCountry]);

    const onClick = async (event: MapMouseEvent) => {
        const feature = event.features?.[0];
        if (!feature) {
            return;
        }
        const clusterId = feature.properties?.cluster_id;

        // If it's a cluster, zoom in to expand it
        if (clusterId) {
            const geojsonSource = mapRef.current?.getSource(sourceId) as GeoJSONSource;
            if (geojsonSource && feature.geometry && 'coordinates' in feature.geometry) {
                try {
                    const zoom = await geojsonSource.getClusterExpansionZoom(clusterId);
                    mapRef.current?.easeTo({
                        center: feature.geometry.coordinates as [number, number],
                        zoom,
                        duration: 500,
                    });
                } catch (error) {
                    console.warn('Could not get cluster expansion zoom:', error);
                }
            }
        }
    };

    if (items.length === 0) {
        return (
            <Card className="w-full p-4">
                <h3 className="text-lg font-semibold mb-4">{title}</h3>
                <div className="text-center py-8 text-muted-fg">
                    {emptyMessage}
                </div>
            </Card>
        );
    }

    const actualCoords = items.filter(item => getCoordinates(item).hasActualCoordinates).length;
    const fallbackCoords = items.length - actualCoords;

    return (
        <Card className="w-full p-4">
            <h3 className="text-lg font-semibold mb-4">{title}</h3>

            {/* Legend */}
            <div className="mb-4 flex flex-wrap items-center gap-4 text-sm text-muted-fg">
                <span>{legendLabel}</span>
                {legend.map(({ color, label }) => (
                    <div key={label} className="flex items-center gap-2">
                        <div className="w-4 h-4 rounded-full" style={{ backgroundColor: color }}></div>
                        <span>{label}</span>
                    </div>
                ))}
                <span className="ml-4">Total: {items.length} {countNoun}</span>
            </div>

            <div className="w-full h-[500px] overflow-hidden rounded-lg">
                <Map
                    initialViewState={{
                        latitude: 20,
                        longitude: 0,
                        zoom: 1.0,
                    }}
                    mapStyle={maplibreStyle}
                    interactiveLayerIds={[clusterLayer.id!]}
                    onClick={onClick}
                    ref={mapRef}
                >
                    <Source
                        id={sourceId}
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
                    <h4 className="text-md font-medium mb-2">{countryStatsTitle}</h4>
                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2 text-sm">
                        {Object.entries(countryData)
                            .sort(([, a], [, b]) => b - a)
                            .slice(0, 8)
                            .map(([country, count]) => (
                                <div key={country} className="flex justify-between bg-muted px-2 py-1 rounded">
                                    <span className="truncate">{country}</span>
                                    <span className="font-medium">{count}</span>
                                </div>
                            ))
                        }
                    </div>

                    {/* Coordinate accuracy note */}
                    <div className="mt-3 text-xs text-muted-fg">
                        {actualCoords > 0 && fallbackCoords > 0
                            ? `📍 ${actualCoords} precise locations, ${fallbackCoords} country-based approximations`
                            : actualCoords > 0
                                ? `📍 All locations show precise coordinates`
                                : `📍 All locations are country-based approximations`}
                    </div>
                </div>
            )}
        </Card>
    );
}
