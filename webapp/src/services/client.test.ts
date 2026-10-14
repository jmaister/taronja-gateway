import { describe, it, expect } from 'vitest';
import { handleResponse } from './client';

describe('handleResponse', () => {
    it('returns the data when there is no error', () => {
        expect(handleResponse<{ id: string }>({ data: { id: 'abc' } })).toEqual({ id: 'abc' });
    });

    it('throws a real Error with the API error message when present', () => {
        expect(() => handleResponse({ error: { message: 'not found' } })).toThrow('not found');
    });

    it('the thrown value is a real Error instance, not the raw {code,message} object', () => {
        // This matters because callers (and TanStack Query's own typing) rely
        // on `error instanceof Error` / `error.message` working consistently —
        // see handleResponse's own doc comment.
        try {
            handleResponse({ error: { message: 'boom' } });
            expect.unreachable('handleResponse should have thrown');
        } catch (err) {
            expect(err).toBeInstanceOf(Error);
        }
    });

    it('falls back to a generic message when the error has none', () => {
        expect(() => handleResponse({ error: {} })).toThrow('Request failed');
    });

    it('returns undefined data as-is when there is no error and no data', () => {
        expect(handleResponse({})).toBeUndefined();
    });
});
