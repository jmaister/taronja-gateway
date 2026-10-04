import { getRequestStatistics, getRequestDetails, getRequestTimeSeries } from '@/apiclient';
import type { RequestStatistics, RequestDetailsResponse, TimeSeriesResponse, TimeSeriesGranularity } from '@/apiclient';
import { useQuery } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';
import { trafficFilterToQuery } from '../lib/trafficFilter';
import type { TrafficFilterState } from '../lib/trafficFilter';

export const statisticsKeys = {
  statistics: (filter: TrafficFilterState) => ['statistics', filter] as const,
  requestDetails: (filter: TrafficFilterState) => ['requestDetails', filter] as const,
  timeSeries: (filter: TrafficFilterState, granularity: TimeSeriesGranularity) =>
    ['timeSeries', filter, granularity] as const,
};

const MINUTE = 60 * 1000;

export function useRequestStatistics(filter: TrafficFilterState) {
  return useQuery({
    queryKey: statisticsKeys.statistics(filter),
    queryFn: async () => {
      const response = await getRequestStatistics({
        query: trafficFilterToQuery(filter),
        client: customApiClient,
      });
      return handleResponse<RequestStatistics>(response);
    },
    staleTime: 5 * MINUTE,
    gcTime: 10 * MINUTE,
  });
}

// Powers "traffic over time" graphs; see doc/middleware/ja4-fingerprint.md for
// what "unique visitors" (fingerprint-based) means.
export function useRequestTimeSeries(filter: TrafficFilterState, granularity: TimeSeriesGranularity) {
  return useQuery({
    queryKey: statisticsKeys.timeSeries(filter, granularity),
    queryFn: async () => {
      const response = await getRequestTimeSeries({
        query: { ...trafficFilterToQuery(filter), granularity },
        client: customApiClient,
      });
      return handleResponse<TimeSeriesResponse>(response);
    },
    staleTime: MINUTE,
    gcTime: 5 * MINUTE,
  });
}

export function useRequestDetails(filter: TrafficFilterState) {
  return useQuery({
    queryKey: statisticsKeys.requestDetails(filter),
    queryFn: async () => {
      const response = await getRequestDetails({
        query: trafficFilterToQuery(filter),
        client: customApiClient,
      });
      return handleResponse<RequestDetailsResponse>(response);
    },
    staleTime: 5 * MINUTE,
    gcTime: 10 * MINUTE,
  });
}
