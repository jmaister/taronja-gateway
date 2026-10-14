import { listTokens, getToken, createToken as apiCreateToken, deleteToken } from '@/apiclient';
import type { TokenCreateRequest, TokenResponse, TokenCreateResponse } from '@/apiclient';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const tokenKeys = {
  root: () => ['tokens'] as const,
  userTokens: (userId: string) => ['users', userId, 'tokens'] as const,
  detail: (tokenId: string) => ['tokens', tokenId] as const,
};

export function useUserTokens(userId: string) {
  return useQuery({
    queryKey: tokenKeys.userTokens(userId),
    queryFn: async () => {
      const response = await listTokens({ path: { userId }, client: customApiClient });
      return handleResponse<TokenResponse[]>(response);
    },
    enabled: !!userId,
  });
}

export function useToken(tokenId: string) {
  return useQuery({
    queryKey: tokenKeys.detail(tokenId),
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
      queryClient.invalidateQueries({ queryKey: tokenKeys.userTokens(userId) });
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
      queryClient.invalidateQueries({ queryKey: tokenKeys.root() });
      // Also invalidate everything under 'users' — that prefix covers
      // tokenKeys.userTokens(userId) (['users', userId, 'tokens']) too, and
      // revoking a token changes that list. A cross-domain invalidation
      // rather than reaching into users.ts's own key builder: it's the
      // 'users' prefix itself that needs invalidating here, not any one
      // user's specific key.
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
  });
}
