import { describe, it, expect } from 'vitest';
import { getCountryCoordinates, countryCoordinates } from './countryCoordinates';

describe('getCountryCoordinates', () => {
    it('returns the known coordinates for an exact country name', () => {
        expect(getCountryCoordinates('Spain')).toEqual([-3.7492, 40.4637]);
    });

    it('falls back to "Unknown" for a name not in the table', () => {
        expect(getCountryCoordinates('Atlantis')).toEqual(countryCoordinates['Unknown']);
    });

    it('falls back to "Unknown" for an empty string', () => {
        expect(getCountryCoordinates('')).toEqual(countryCoordinates['Unknown']);
    });

    it('is case-sensitive — a differently-cased known name still falls back', () => {
        // The table only has exact entries ("United States", not "united states");
        // this locks in that behavior rather than silently changing it later.
        expect(getCountryCoordinates('united states')).toEqual(countryCoordinates['Unknown']);
    });

    it('every table entry is a valid [longitude, latitude] pair', () => {
        for (const [name, [lon, lat]] of Object.entries(countryCoordinates)) {
            expect(lon, `${name}'s longitude`).toBeGreaterThanOrEqual(-180);
            expect(lon, `${name}'s longitude`).toBeLessThanOrEqual(180);
            expect(lat, `${name}'s latitude`).toBeGreaterThanOrEqual(-90);
            expect(lat, `${name}'s latitude`).toBeLessThanOrEqual(90);
        }
    });
});
