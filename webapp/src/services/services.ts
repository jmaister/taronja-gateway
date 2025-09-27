import {
  getUserCounters,
  getUserCounterHistory,
  adjustUserCounters,
  getAllUserCounters,
  getAvailableCounters,
} from '@/apiclient/sdk.gen';
import type {
  UserCountersResponse,
  CounterHistoryResponse,
  CounterAdjustmentRequest,
  AllUserCountersResponse,
  AvailableCountersResponse,
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
    // Throw the error string directly so it can be shown in the UI
    throw response.error;
  }
  return response.data as T;
}

// Query Keys
export const queryKeys = {
  users: () => ['users'] as const,
  user: (id: string) => ['users', id] as const,
  currentUser: () => ['currentUser'] as const,
  statistics: (startDate?: string, endDate?: string) => ['statistics', { startDate, endDate }] as const,
  requestDetails: (startDate: string, endDate: string) => ['requestDetails', { startDate, endDate }] as const,
  userTokens: (userId: string) => ['users', userId, 'tokens'] as const,
  token: (tokenId: string) => ['tokens', tokenId] as const,
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
      return response.data;
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

export function useRequestDetails(startDate: string, endDate: string) {
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
      await deleteToken({ path: { tokenId }, client: customApiClient });
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
