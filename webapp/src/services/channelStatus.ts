// Every notification channel this gateway has configured, and the current
// user's status on each — the generic counterpart to services/telegram.ts's
// Telegram-specific status: a channel that works for every user the
// moment it's configured (e.g. email) is "active"; a channel that also
// requires this specific user to individually link their account first
// (e.g. telegram) is "connected"/"not_connected" instead. See
// notification.Service.ChannelStatuses on the backend, which this exposes.
import { getChannelStatuses } from '@/apiclient';
import type { GetChannelStatusesResponse } from '@/apiclient';
import { useQuery } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const channelStatusKeys = {
  all: () => ['channels', 'status'] as const,
};

export function useChannelStatuses() {
  return useQuery({
    queryKey: channelStatusKeys.all(),
    queryFn: async (): Promise<GetChannelStatusesResponse> => {
      const response = await getChannelStatuses({ client: customApiClient });
      return handleResponse<GetChannelStatusesResponse>(response);
    },
  });
}
