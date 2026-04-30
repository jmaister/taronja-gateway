import { describe, expect, it, vi } from 'vitest';
import { TaronjaGatewayError } from './errors';
import { createTaronjaClient } from './client';

describe('createTaronjaClient', () => {
    it('builds the login url against the configured base url', () => {
        const client = createTaronjaClient({
            baseUrl: '/gateway',
            fetch: vi.fn(),
        });

        expect(client.getLoginUrl({ redirectTo: '/_/admin/home' })).toBe('/gateway/login?redirect=%2F_%2Fadmin%2Fhome');
    });

    it('returns null for an unauthenticated current user request', async () => {
        const client = createTaronjaClient({
            fetch: vi.fn().mockResolvedValue(new Response(null, { status: 401 })),
        });

        await expect(client.getCurrentUser()).resolves.toBeNull();
    });

    it('throws a typed error for failed requests', async () => {
        const client = createTaronjaClient({
            fetch: vi.fn().mockResolvedValue(new Response(JSON.stringify({ message: 'boom', code: 500 }), {
                status: 500,
                headers: {
                    'Content-Type': 'application/json',
                },
            })),
        });

        await expect(client.getHealth()).rejects.toBeInstanceOf(TaronjaGatewayError);
    });
});
