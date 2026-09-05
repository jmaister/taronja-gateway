import { describe, it, expect } from 'vitest';
import { cn } from './cn';

describe('cn', () => {
    it('joins truthy class names with a space', () => {
        expect(cn('a', 'b', 'c')).toBe('a b c');
    });

    it('drops undefined, null, false, and empty strings', () => {
        expect(cn('a', undefined, 'b', null, false, 'c')).toBe('a b c');
    });

    it('returns an empty string when nothing is truthy', () => {
        expect(cn(undefined, null, false)).toBe('');
    });

    it('returns an empty string when called with no arguments', () => {
        expect(cn()).toBe('');
    });

    it('preserves argument order', () => {
        expect(cn('z', 'a', 'm')).toBe('z a m');
    });
});
