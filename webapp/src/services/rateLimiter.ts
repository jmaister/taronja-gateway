import { getRateLimiterStats, getRateLimiterConfig, getBlockedClients } from '@/apiclient/sdk.gen';
import type { RateLimiterStats, RateLimiterConfigResponse, BlockedClientsResponse } from '@/apiclient/types.gen';
import { useQuery } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const rateLimiterKeys = {
  stats: () => ['rateLimiterStats'] as const,
  config: () => ['rateLimiterConfig'] as const,
  blockedClients: (ip: string | undefined, limit: number, offset: number) =>
    ['blockedClients', { ip, limit, offset }] as const,
};

export function useRateLimiterStats() {
  return useQuery<RateLimiterStats, Error>({
    queryKey: rateLimiterKeys.stats(),
    queryFn: async () => {
      const response = await getRateLimiterStats({ client: customApiClient });
      return handleResponse<RateLimiterStats>(response);
    },
    staleTime: 10_000, // refresh every 10 seconds
    gcTime: 60_000,
  });
}

export function useRateLimiterConfig() {
  return useQuery<RateLimiterConfigResponse, Error>({
    queryKey: rateLimiterKeys.config(),
    queryFn: async () => {
      const response = await getRateLimiterConfig({ client: customApiClient });
      return handleResponse<RateLimiterConfigResponse>(response);
    },
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });
}

// Persistent history of rate-limiter block events (db.BlockedClient) — see
// doc/middleware/rate-limiter.md. Distinct from useRateLimiterStats, which
// only reflects whatever the in-memory limiter still happens to be
// tracking right now; this survives past that cleanup.
export function useBlockedClients(ip?: string, limit = 50, offset = 0) {
  return useQuery<BlockedClientsResponse, Error>({
    queryKey: rateLimiterKeys.blockedClients(ip, limit, offset),
    queryFn: async () => {
      const response = await getBlockedClients({
        query: { ip, limit, offset },
        client: customApiClient,
      });
      return handleResponse<BlockedClientsResponse>(response);
    },
    staleTime: 10_000,
    gcTime: 60_000,
  });
}
