import {
  getUserCounterHistory,
  adjustUserCounters,
  getAllUserCounters,
  getAvailableCounters,
  getRateLimiterStats,
  getRateLimiterConfig,
  getMiddlewareStatus,
  getAllMiddlewareMetrics,
} from '@/apiclient/sdk.gen';
import type {
  CounterHistoryResponse,
  CounterAdjustmentRequest,
  AllUserCountersResponse,
  AvailableCountersResponse,
  RateLimiterStats,
  RateLimiterConfigResponse,
  MiddlewareStatusList,
  MiddlewareMetricsList,
} from '@/apiclient/types.gen';


import { 
  TokenCreateRequest, 
  UserCreateRequest, 
  UserResponse, 
  listUsers,
  getUserById,
  getCurrentUser,
  createUser as apiCreateUser,
  getRequestStatistics,
  getRequestDetails,
  listTokens,
  getToken,
  createToken as apiCreateToken,
  deleteToken,
  TokenResponse,
  RequestStatistics,
  RequestDetailsResponse,
  TokenCreateResponse
} from '@/apiclient';
import { createClient } from '@/apiclient/client';
import { QueryClient, useQuery, useMutation, useQueryClient } from '@tanstack/react-query';

export interface CurrentUser extends UserResponse {}

export const customApiClient = createClient({
    baseUrl: "/_",
})

// Create QueryClient
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

// Helper function to handle responses
// Helper to handle API responses: throws on error, returns data
function handleResponse<T = any>(response: any): T {
  if (response && response.error) {
    // Throw a real Error (not the raw { code, message } API error object)
    // so `error instanceof Error` checks and String(error) work consistently
    // wherever a query/mutation's error is rendered, not just the call
    // sites that know to reach for `.message` explicitly.
    throw new Error(response.error.message || 'Request failed');
  }
  return response.data as T;
}

// Query Keys
export const queryKeys = {
  users: () => ['users'] as const,
  user: (id: string) => ['users', id] as const,
  currentUser: () => ['currentUser'] as const,
  statistics: (startDate?: string, endDate?: string) => ['statistics', { startDate, endDate }] as const,
  requestDetails: (startDate: string, endDate: string, isStatic?: boolean) =>
    ['requestDetails', { startDate, endDate, isStatic }] as const,
  userTokens: (userId: string) => ['users', userId, 'tokens'] as const,
  token: (tokenId: string) => ['tokens', tokenId] as const,
  rateLimiterStats: () => ['rateLimiterStats'] as const,
  rateLimiterConfig: () => ['rateLimiterConfig'] as const,
  middlewareStatus: () => ['middlewareStatus'] as const,
  middlewareMetrics: () => ['middlewareMetrics'] as const,
} as const;

// Users hooks
export function useUsers() {
  return useQuery({
    queryKey: queryKeys.users(),
    queryFn: async () => {
      const response = await listUsers({ client: customApiClient });
      return handleResponse<UserResponse[]>(response);
    },
  });
}

export function useUser(userId: string) {
  return useQuery({
    queryKey: queryKeys.user(userId),
    queryFn: async () => {
      const response = await getUserById({ path: { userId }, client: customApiClient });
      return handleResponse<UserResponse>(response);
    },
    enabled: !!userId,
  });
}

export function useCurrentUser() {
  return useQuery({
    queryKey: queryKeys.currentUser(),
    queryFn: async () => {
      const response = await getCurrentUser({ client: customApiClient });
      return handleResponse<CurrentUser>(response);
    },
  });
}

export function useCreateUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (userData: UserCreateRequest) => {
      const response = await apiCreateUser({ body: userData, client: customApiClient });
      return handleResponse<UserResponse>(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.users() });
    },
  });
}

// Statistics hooks
export function useRequestStatistics(startDate?: string, endDate?: string) {
  return useQuery({
    queryKey: queryKeys.statistics(startDate, endDate),
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

// isStatic filters by whether the request was for a static asset (see
// management.excludeStaticAssets): true for only static-asset requests,
// false for only non-static requests, undefined for both.
export function useRequestDetails(startDate: string, endDate: string, isStatic?: boolean) {
  return useQuery({
    queryKey: queryKeys.requestDetails(startDate, endDate, isStatic),
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

// Token hooks
export function useUserTokens(userId: string) {
  return useQuery({
    queryKey: queryKeys.userTokens(userId),
    queryFn: async () => {
      const response = await listTokens({ path: { userId }, client: customApiClient });
      return handleResponse<TokenResponse[]>(response);
    },
    enabled: !!userId,
  });
}

export function useToken(tokenId: string) {
  return useQuery({
    queryKey: queryKeys.token(tokenId),
    queryFn: async () => {
      const response = await getToken({ path: { tokenId }, client: customApiClient });
      return handleResponse<TokenResponse>(response);
    },
    enabled: !!tokenId,
  });
}

export function useCreateToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ userId, tokenData }: { userId: string; tokenData: TokenCreateRequest }) => {
      const response = await apiCreateToken({ 
        path: { userId }, 
        body: tokenData,
        client: customApiClient,
      });
      return handleResponse<TokenCreateResponse>(response);
    },
    onSuccess: (_, { userId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.userTokens(userId) });
    },
  });
}

export function useRevokeToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (tokenId: string) => {
      const response = await deleteToken({ path: { tokenId }, client: customApiClient });
      handleResponse(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tokens'] });
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });
}

// Counters hooks
export function useAllUserCounters(counterId: string) {
  return useQuery<AllUserCountersResponse, Error>({
    queryKey: ['counters', 'allUsers', counterId],
    queryFn: async () => {
      const response = await getAllUserCounters({ path: { counterId }, client: customApiClient });
      return handleResponse<AllUserCountersResponse>(response);
    },
    enabled: !!counterId,
    staleTime: 60_000,
  });
}

export function useCounterHistory(counterId: string, userId: string | null) {
  return useQuery<CounterHistoryResponse, Error>({
    queryKey: ['counters', 'history', counterId, userId],
    queryFn: async () => {
      if (!userId) throw new Error('No user selected');
      const response = await getUserCounterHistory({ path: { counterId, userId }, client: customApiClient });
      return handleResponse<CounterHistoryResponse>(response);
    },
    enabled: !!userId && !!counterId,
    staleTime: 30_000,
  });
}

export function useAdjustCounters() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ counterId, userId, adjustment }: { counterId: string; userId: string; adjustment: CounterAdjustmentRequest }) => {
      const response = await adjustUserCounters({ path: { counterId, userId }, body: adjustment, client: customApiClient });
      if (response.error) throw new Error(response.error.message);
      return response.data;
    },
    onSuccess: (_, { counterId, userId }) => {
      queryClient.invalidateQueries({ queryKey: ['counters', 'allUsers', counterId] });
      queryClient.invalidateQueries({ queryKey: ['counters', 'history', counterId, userId] });
      queryClient.invalidateQueries({ queryKey: ['counters', 'available'] });
    },
  });
}

export function useAvailableCounters() {
  return useQuery<AvailableCountersResponse, Error>({
    queryKey: ['counters', 'available'],
    queryFn: async () => {
      const response = await getAvailableCounters({ client: customApiClient });
      return handleResponse<AvailableCountersResponse>(response);
    },
    staleTime: 5 * 60 * 1000, // 5 minutes - counter types don't change often
    gcTime: 10 * 60 * 1000, // 10 minutes
  });
}

export function useRateLimiterStats() {
  return useQuery<RateLimiterStats, Error>({
    queryKey: queryKeys.rateLimiterStats(),
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
    queryKey: queryKeys.rateLimiterConfig(),
    queryFn: async () => {
      const response = await getRateLimiterConfig({ client: customApiClient });
      return handleResponse<RateLimiterConfigResponse>(response);
    },
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });
}

// Middleware status/health/metrics hooks (see doc/refactor01.md Phases 3 & 5)
export function useMiddlewareStatus() {
  return useQuery<MiddlewareStatusList, Error>({
    queryKey: queryKeys.middlewareStatus(),
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
    queryKey: queryKeys.middlewareMetrics(),
    queryFn: async () => {
      const response = await getAllMiddlewareMetrics({ client: customApiClient });
      return handleResponse<MiddlewareMetricsList>(response);
    },
    staleTime: 10_000, // refresh every 10 seconds, same cadence as rate limiter stats
    gcTime: 60_000,
  });
}
