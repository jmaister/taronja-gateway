import { getMiddlewareStatus, getAllMiddlewareMetrics } from '@/apiclient/sdk.gen';
import type { MiddlewareStatusList, MiddlewareMetricsList } from '@/apiclient/types.gen';
import { useQuery } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

// Middleware status/health/metrics hooks (see doc/refactor01.md Phases 3 & 5)
export const middlewareKeys = {
  status: () => ['middlewareStatus'] as const,
  metrics: () => ['middlewareMetrics'] as const,
};

export function useMiddlewareStatus() {
  return useQuery<MiddlewareStatusList, Error>({
    queryKey: middlewareKeys.status(),
    queryFn: async () => {
      const response = await getMiddlewareStatus({ client: customApiClient });
      return handleResponse<MiddlewareStatusList>(response);
    },
    staleTime: 30_000,
    gcTime: 60_000,
  });
}

export function useMiddlewareMetrics() {
  return useQuery<MiddlewareMetricsList, Error>({
    queryKey: middlewareKeys.metrics(),
    queryFn: async () => {
      const response = await getAllMiddlewareMetrics({ client: customApiClient });
      return handleResponse<MiddlewareMetricsList>(response);
    },
    staleTime: 10_000, // refresh every 10 seconds, same cadence as rate limiter stats
    gcTime: 60_000,
  });
}
