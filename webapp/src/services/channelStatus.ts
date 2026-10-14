// Every notification channel this gateway has configured, and the current
// user's status on each — the generic counterpart to services/telegram.ts's
// Telegram-specific status: a channel that works for every user the
// moment it's configured (e.g. email) is "active"; a channel that also
// requires this specific user to individually link their account first
// (e.g. telegram) is "connected"/"not_connected" instead. See
// notification.Service.ChannelStatuses on the backend, which this exposes.
import { getChannelStatuses, getUserChannelStatuses, getUserNotificationPreference, getUserTelegramLinkCode } from '@/apiclient';
import type {
  GetChannelStatusesResponse,
  GetUserChannelStatusesResponse,
  GetUserNotificationPreferenceResponse,
  GetUserTelegramLinkCodeResponse,
} from '@/apiclient';
import { useQuery, useMutation } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const channelStatusKeys = {
  all: () => ['channels', 'status'] as const,
  forUser: (userId: string) => ['users', userId, 'channels', 'status'] as const,
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

// useUserChannelStatuses is useChannelStatuses' admin-facing counterpart —
// same shape, but for a user given by ID (a user detail page) rather than
// the caller's own session. Requires the caller to be an admin; the
// backend 401s otherwise.
export function useUserChannelStatuses(userId: string) {
  return useQuery({
    queryKey: channelStatusKeys.forUser(userId),
    queryFn: async (): Promise<GetUserChannelStatusesResponse> => {
      const response = await getUserChannelStatuses({ path: { userId }, client: customApiClient });
      return handleResponse<GetUserChannelStatusesResponse>(response);
    },
    enabled: !!userId,
  });
}

export const notificationPreferenceKeys = {
  forUser: (userId: string) => ['users', userId, 'notifications', 'preference'] as const,
};

// useUserNotificationPreference reads a user's preferred delivery channel
// (see NotificationPreference) — the admin-facing counterpart to
// services/... there's no self-service equivalent hook yet since no UI
// consumes GET/PUT /api/notifications/preferences for the caller's own
// account either.
export function useUserNotificationPreference(userId: string) {
  return useQuery({
    queryKey: notificationPreferenceKeys.forUser(userId),
    queryFn: async (): Promise<GetUserNotificationPreferenceResponse> => {
      const response = await getUserNotificationPreference({ path: { userId }, client: customApiClient });
      return handleResponse<GetUserNotificationPreferenceResponse>(response);
    },
    enabled: !!userId,
  });
}

// useGenerateUserTelegramLinkCode is the admin-facing counterpart to
// services/telegram.ts's useCreateTelegramLinkCode: generates a fresh
// Telegram deep link for a user given by ID, so an admin can hand it to
// that user directly (message, email, ...) instead of the user
// generating their own from a self-service page. A mutation, not a
// query, for the same reason as the self-service version — every call
// issues a fresh, single-use, short-lived code.
export function useGenerateUserTelegramLinkCode() {
  return useMutation({
    mutationFn: async (userId: string): Promise<GetUserTelegramLinkCodeResponse> => {
      const response = await getUserTelegramLinkCode({ path: { userId }, client: customApiClient });
      return handleResponse<GetUserTelegramLinkCodeResponse>(response);
    },
  });
}
