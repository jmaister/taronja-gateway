import { listUsers, getUserById, createUser as apiCreateUser } from '@/apiclient';
import type { UserCreateRequest, UserResponse } from '@/apiclient';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const userKeys = {
  all: () => ['users'] as const,
  detail: (id: string) => ['users', id] as const,
};

export function useUsers() {
  return useQuery({
    queryKey: userKeys.all(),
    queryFn: async () => {
      const response = await listUsers({ client: customApiClient });
      return handleResponse<UserResponse[]>(response);
    },
  });
}

export function useUser(userId: string) {
  return useQuery({
    queryKey: userKeys.detail(userId),
    queryFn: async () => {
      const response = await getUserById({ path: { userId }, client: customApiClient });
      return handleResponse<UserResponse>(response);
    },
    enabled: !!userId,
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
      queryClient.invalidateQueries({ queryKey: userKeys.all() });
    },
  });
}
