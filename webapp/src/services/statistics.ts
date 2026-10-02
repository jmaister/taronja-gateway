import { getRequestStatistics, getRequestDetails, getRequestTimeSeries } from '@/apiclient';
import type { RequestStatistics, RequestDetailsResponse, TimeSeriesResponse, TimeSeriesGranularity } from '@/apiclient';
import { useQuery } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const statisticsKeys = {
  statistics: (startDate?: string, endDate?: string) => ['statistics', { startDate, endDate }] as const,
  requestDetails: (startDate: string, endDate: string, isStatic?: boolean) =>
    ['requestDetails', { startDate, endDate, isStatic }] as const,
  timeSeries: (startDate: string, endDate: string, granularity: TimeSeriesGranularity) =>
    ['timeSeries', { startDate, endDate, granularity }] as const,
};

export function useRequestStatistics(startDate?: string, endDate?: string) {
  return useQuery({
    queryKey: statisticsKeys.statistics(startDate, endDate),
    queryFn: async () => {
      const response = await getRequestStatistics({
        query: {
          start_date: startDate,
          end_date: endDate,
        },
        client: customApiClient,
      });
      return handleResponse<RequestStatistics>(response);
    },
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });
}

// Powers "traffic over time" style graphs (requests/unique visitors/errors
// per minute, hour, day, week, or month) — see doc/middleware/ja4-fingerprint.md
// for what "unique visitors" (fingerprint-based) actually means here.
export function useRequestTimeSeries(startDate: string, endDate: string, granularity: TimeSeriesGranularity) {
  return useQuery({
    queryKey: statisticsKeys.timeSeries(startDate, endDate, granularity),
    queryFn: async () => {
      const response = await getRequestTimeSeries({
        query: {
          start_date: startDate,
          end_date: endDate,
          granularity,
        },
        client: customApiClient,
      });
      return handleResponse<TimeSeriesResponse>(response);
    },
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

// isStatic filters by whether the request was for a static asset (see
// management.excludeStaticAssets): true for only static-asset requests,
// false for only non-static requests, undefined for both.
export function useRequestDetails(startDate: string, endDate: string, isStatic?: boolean) {
  return useQuery({
    queryKey: statisticsKeys.requestDetails(startDate, endDate, isStatic),
    queryFn: async () => {
      const response = await getRequestDetails({
        query: {
          start_date: startDate,
          end_date: endDate,
          is_static: isStatic,
        },
        client: customApiClient,
      });
      return handleResponse<RequestDetailsResponse>(response);
    },
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });
}
