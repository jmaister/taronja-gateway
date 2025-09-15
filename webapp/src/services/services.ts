import { QueryClient, useQuery } from '@tanstack/react-query';
import { fetchRequestStatistics } from './api';
import { getRequestDetails, RequestDetailsResponse } from '@/apiclient';
import { createClient } from '@/apiclient/client';


// Create a client
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,      
    },
  },
});

// TODO: this should be taken from config.Management.Prefix at runtime
// mignt not be possible because we have to get that info from the server
// at build time we don't have that info
// for now we assume the prefix is always "/_"
const customApiClient = createClient({
    baseUrl: '/_',
});


// Helper to handle API responses: throws on error, returns data
function handleApiResponse<T = any>(response: any): T {
  if (response && response.error) {
    // Throw the error string directly so it can be shown in the UI
    throw new Error(response.error);
  }
  
  // Handle different response structures
  if (response && response.data !== undefined) {
    return response.data as T;
  }
  
  // If response itself is the data (direct response)
  return response as T;
}


// Query Keys
export const queryKeys = {
    statistics: (startDate?: string, endDate?: string) => ['statistics', { startDate, endDate }] as const,
    requestDetails: (startDate?: string, endDate?: string) => ['requestDetails', { startDate, endDate }] as const,
} as const;

// TanStack Query hooks
export function useRequestStatistics(startDate?: string, endDate?: string) {
    return useQuery({
        queryKey: queryKeys.statistics(startDate, endDate),
        queryFn: () => fetchRequestStatistics(startDate, endDate),
        staleTime: 5 * 60 * 1000, // Consider data fresh for 5 minutes
        gcTime: 10 * 60 * 1000, // Keep in cache for 10 minutes
    });
}

export function useRequestDetails(startDate?: string, endDate?: string) {
    return useQuery({
        queryKey: queryKeys.requestDetails(startDate, endDate),
        queryFn: async () => {
            const response = await getRequestDetails({
                query: {
                    start_date: startDate,
                    end_date: endDate,
                },
                client: customApiClient,
            });
            
            return handleApiResponse<RequestDetailsResponse>(response);
        },
        staleTime: 5 * 60 * 1000, // Consider data fresh for 5 minutes
        gcTime: 10 * 60 * 1000, // Keep in cache for 10 minutes
    });
}
