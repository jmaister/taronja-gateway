export function getSafeLocale(): string | undefined {
    if (typeof navigator === 'undefined') {
        return undefined;
    }

    const candidates = [navigator.language, ...(navigator.languages || [])]
        .filter((value): value is string => typeof value === 'string' && value.trim().length > 0);

    for (const locale of candidates) {
        try {
            // Some browsers/extensions can provide non-BCP47 values (e.g. "chrome://..."),
            // which will throw RangeError in Intl constructors.
            if (Intl.getCanonicalLocales(locale).length > 0) {
                return locale;
            }
        } catch {
            // ignore invalid locale
        }
    }

    return undefined;
}
