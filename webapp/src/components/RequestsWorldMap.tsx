import { ComposableMap, Geographies, Geography } from "react-simple-maps";
import { useState } from "react";
import { RequestDetail } from "./RequestsDetailsTable";

const geoUrl = "https://cdn.jsdelivr.net/npm/world-atlas@2/countries-110m.json";

interface RequestsWorldMapProps {
    requests: RequestDetail[];
}

export function RequestsWorldMap({ requests }: RequestsWorldMapProps) {
    const [tooltip, setTooltip] = useState<string | null>(null);
    
    // Count requests by country
    const countryData = requests.reduce((acc, request) => {
        const country = request.country || "Unknown";
        acc[country] = (acc[country] || 0) + 1;
        return acc;
    }, {} as Record<string, number>);

    // Get max count for color scaling
    const maxCount = Math.max(...Object.values(countryData), 1);

    // Function to get color intensity based on request count
    const getCountryColor = (geoName: string) => {
        const count = countryData[geoName] || 0;
        if (count === 0) return "#f3f4f6"; // gray-100 for no requests
        
        const intensity = count / maxCount;
        // Scale from light blue to dark blue
        const alpha = Math.max(0.2, intensity);
        return `rgba(59, 130, 246, ${alpha})`; // blue-500 with varying alpha
    };

    if (requests.length === 0) {
        return (
            <div className="w-full bg-white border border-gray-200 rounded-lg p-4">
                <h3 className="text-lg font-semibold mb-4">Requests by Country</h3>
                <div className="text-center py-8 text-gray-500">
                    No request data available for the selected period
                </div>
            </div>
        );
    }

    return (
        <div className="w-full bg-white border border-gray-200 rounded-lg p-4 relative">
            <h3 className="text-lg font-semibold mb-4">Requests by Country</h3>
            
            {/* Tooltip */}
            {tooltip && (
                <div className="absolute top-2 right-2 z-10 bg-gray-900 text-white text-sm py-2 px-3 rounded shadow-lg pointer-events-none max-w-xs">
                    {tooltip}
                </div>
            )}
            
            {/* Legend */}
            <div className="mb-4 flex flex-wrap items-center gap-4 text-sm text-gray-600">
                <span>Low</span>
                <div className="flex gap-1">
                    <div className="w-4 h-4 bg-blue-200 rounded"></div>
                    <div className="w-4 h-4 bg-blue-300 rounded"></div>
                    <div className="w-4 h-4 bg-blue-400 rounded"></div>
                    <div className="w-4 h-4 bg-blue-500 rounded"></div>
                </div>
                <span>High</span>
                <span className="ml-4">Total: {requests.length} requests</span>
            </div>

            {/* Map */}
            <div className="w-full overflow-hidden">
                <ComposableMap
                    projection="geoMercator"
                    projectionConfig={{
                        scale: 140,
                        center: [0, 20]
                    }}
                    width={800}
                    height={400}
                    style={{ width: "100%", height: "auto", maxHeight: "400px" }}
                >
                    <Geographies geography={geoUrl}>
                        {({ geographies }: { geographies: any[] }) =>
                            geographies.map((geo: any) => {
                                // Get country name from available properties
                                const props = geo.properties || {};
                                const countryName = props.NAME || 
                                                  props.NAME_LONG || 
                                                  props.ADMIN || 
                                                  props.name ||
                                                  props.NAME_EN ||
                                                  props.COUNTRY ||
                                                  props.sovereignt ||
                                                  props.NAME_SORT ||
                                                  'Unknown Country';
                                
                                const requestCount = countryData[countryName] || 0;
                                
                                return (
                                    <Geography
                                        key={geo.rsmKey}
                                        geography={geo}
                                        fill={getCountryColor(countryName)}
                                        stroke="#e5e7eb"
                                        strokeWidth={0.5}
                                        style={{
                                            default: { outline: "none" },
                                            hover: { 
                                                outline: "none",
                                                fill: "#3b82f6",
                                                cursor: "pointer"
                                            },
                                            pressed: { outline: "none" }
                                        }}
                                        onMouseEnter={() => {
                                            // Use the same country name logic as above
                                            const displayName = countryName || 'Unknown Country';
                                            const tooltipContent = requestCount > 0 
                                                ? `${displayName}: ${requestCount} requests`
                                                : `${displayName}: No requests`;
                                            setTooltip(tooltipContent);
                                        }}
                                        onMouseLeave={() => {
                                            setTooltip(null);
                                        }}
                                    />
                                );
                            })
                        }
                    </Geographies>
                </ComposableMap>
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
                </div>
            )}
        </div>
    );
}
