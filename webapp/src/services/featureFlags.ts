import { listFeatureFlags, createFeatureFlag, updateFeatureFlag, deleteFeatureFlag } from '@/apiclient';
import type { FeatureFlag, FeatureFlagCreateRequest, FeatureFlagUpdateRequest } from '@/apiclient';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const featureFlagKeys = {
  all: () => ['featureFlags'] as const,
};

export function useFeatureFlags() {
  return useQuery({
    queryKey: featureFlagKeys.all(),
    queryFn: async () => {
      const response = await listFeatureFlags({ client: customApiClient });
      return handleResponse<FeatureFlag[]>(response);
    },
  });
}

export function useCreateFeatureFlag() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: FeatureFlagCreateRequest) => {
      const response = await createFeatureFlag({ body, client: customApiClient });
      return handleResponse<FeatureFlag>(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: featureFlagKeys.all() });
    },
  });
}

export function useUpdateFeatureFlag() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ key, body }: { key: string; body: FeatureFlagUpdateRequest }) => {
      const response = await updateFeatureFlag({ path: { key }, body, client: customApiClient });
      return handleResponse<FeatureFlag>(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: featureFlagKeys.all() });
    },
  });
}

export function useDeleteFeatureFlag() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (key: string) => {
      const response = await deleteFeatureFlag({ path: { key }, client: customApiClient });
      handleResponse(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: featureFlagKeys.all() });
    },
  });
}
