import { getUserCounterHistory, adjustUserCounters, getAllUserCounters, getAvailableCounters } from '@/apiclient/sdk.gen';
import type { CounterHistoryResponse, CounterAdjustmentRequest, CounterTransactionResponse, AllUserCountersResponse, AvailableCountersResponse } from '@/apiclient/types.gen';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

// A local key builder, same as every other domain file — previously these
// four hooks built raw array literals inline instead, the one inconsistency
// in an otherwise uniform pattern across the file this was split out of.
export const counterKeys = {
  allUsers: (counterId: string) => ['counters', 'allUsers', counterId] as const,
  history: (counterId: string, userId: string | null) => ['counters', 'history', counterId, userId] as const,
  available: () => ['counters', 'available'] as const,
};

export function useAllUserCounters(counterId: string) {
  return useQuery<AllUserCountersResponse, Error>({
    queryKey: counterKeys.allUsers(counterId),
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
    queryKey: counterKeys.history(counterId, userId),
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
      return handleResponse<CounterTransactionResponse>(response);
    },
    onSuccess: (_, { counterId, userId }) => {
      queryClient.invalidateQueries({ queryKey: counterKeys.allUsers(counterId) });
      queryClient.invalidateQueries({ queryKey: counterKeys.history(counterId, userId) });
      queryClient.invalidateQueries({ queryKey: counterKeys.available() });
    },
  });
}

export function useAvailableCounters() {
  return useQuery<AvailableCountersResponse, Error>({
    queryKey: counterKeys.available(),
    queryFn: async () => {
      const response = await getAvailableCounters({ client: customApiClient });
      return handleResponse<AvailableCountersResponse>(response);
    },
    staleTime: 5 * 60 * 1000, // 5 minutes - counter types don't change often
    gcTime: 10 * 60 * 1000, // 10 minutes
  });
}
