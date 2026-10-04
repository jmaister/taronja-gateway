import { describe, expect, it } from 'vitest';
import { formatBytes, formatDate, formatDateTime } from './format';

describe('formatBytes', () => {
    it('scales units', () => {
        expect(formatBytes(0)).toBe('0 B');
        expect(formatBytes(512)).toBe('512 B');
        expect(formatBytes(1536)).toBe('1.5 KB');
        expect(formatBytes(5 * 1024 * 1024)).toBe('5 MB');
    });
});

describe('date formatting', () => {
    it('returns the fallback for empty or invalid values', () => {
        expect(formatDateTime(null)).toBe('Never');
        expect(formatDate(undefined, '-')).toBe('-');
        expect(formatDateTime('not a date')).toBe('Never');
    });

    it('formats valid values', () => {
        expect(formatDateTime('2024-03-05T10:00:00Z')).not.toBe('Never');
    });
});
