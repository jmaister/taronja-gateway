# Assets

This folder contains static assets used by the webapp.

## MapLibre Style (maplibre-style.json)

This file contains a local copy of the MapLibre style configuration that was previously loaded from `https://demotiles.maplibre.org/style.json`.

### Benefits of Local Caching:
- **Improved Performance**: No external network request needed to load the map style
- **Reliability**: App works even if the external service is unavailable
- **Offline Support**: Map styling works without internet connection
- **Better User Experience**: Faster map loading times

### Usage:
The style is imported and used in `RequestsWorldMap.tsx` component:

```typescript
import maplibreStyleJson from "../assets/maplibre-style.json";
import type { StyleSpecification } from "maplibre-gl";

const maplibreStyle = maplibreStyleJson as StyleSpecification;
```

### Updating:
If you need to update the style, you can:
1. Fetch the latest version from `https://demotiles.maplibre.org/style.json`
2. Replace the content of `maplibre-style.json`
3. Ensure the version property is set to exactly `8` for MapLibre compatibility
