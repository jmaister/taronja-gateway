import { describe, it, expect, vi, afterEach } from 'vitest';
import { getSafeLocale } from './intl';

// getSafeLocale reads the real global `navigator`, so each test stubs it
// directly rather than relying on jsdom's own default locale (which isn't
// this test's concern, and isn't guaranteed stable across jsdom versions).
describe('getSafeLocale', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    it('returns navigator.language when it is a valid BCP47 locale', () => {
        vi.stubGlobal('navigator', { language: 'es-ES', languages: ['es-ES', 'en-US'] });
        expect(getSafeLocale()).toBe('es-ES');
    });

    it('falls through to the first valid entry in navigator.languages when navigator.language is invalid', () => {
        // BCP47 syntax is more permissive than it looks — plain hyphenated
        // alphanumeric strings often parse as "valid", just not any real
        // language. "chrome://..." (a real value some browsers/extensions
        // have been seen to report, per getSafeLocale's own doc comment)
        // has characters BCP47 flatly rejects, so it's a reliable invalid case.
        vi.stubGlobal('navigator', { language: 'chrome://not-a-locale', languages: ['still://bad', 'en-GB'] });
        expect(getSafeLocale()).toBe('en-GB');
    });

    it('returns undefined when no candidate is a valid locale', () => {
        vi.stubGlobal('navigator', { language: 'chrome://not-a-locale', languages: ['still://bad'] });
        expect(getSafeLocale()).toBeUndefined();
    });

    it('returns undefined when navigator.language/languages are both empty', () => {
        vi.stubGlobal('navigator', { language: '', languages: [] });
        expect(getSafeLocale()).toBeUndefined();
    });

    it('returns undefined when navigator itself is undefined', () => {
        vi.stubGlobal('navigator', undefined);
        expect(getSafeLocale()).toBeUndefined();
    });
});
