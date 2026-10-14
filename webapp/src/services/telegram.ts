// User-facing Telegram connection state for the current session's account:
// whether it's linked, and — while a fresh deep link is being shown — an
// actionable way to generate one and to disconnect again. Kept separate
// from notification "preferences" (services/... doesn't have one of those
// yet either) since this is specifically about the Telegram channel link
// itself, not which channel a user prefers.
import { getTelegramLinkCode, getTelegramLinkStatus, unlinkTelegramChat } from '@/apiclient';
import type { GetTelegramLinkCodeResponse, GetTelegramLinkStatusResponse } from '@/apiclient';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const telegramKeys = {
  status: () => ['telegram', 'status'] as const,
};

// TelegramStatus adds `configured` on top of the raw API response:
// `linked` alone can't distinguish "this gateway doesn't have Telegram
// delivery set up at all" (the API returns 503 for every Telegram
// endpoint in that case) from "it's set up, this user just hasn't
// connected yet" — the UI needs to tell those apart to show "Telegram
// notifications aren't available here" instead of a "Connect" button
// that would just fail.
export interface TelegramStatus {
  configured: boolean;
  linked: boolean;
  linkedAt?: string | null;
}

// pollIntervalWhileConnecting is how often useTelegramLinkStatus re-checks
// while a caller is actively waiting for the user to finish the Telegram
// deep-link flow (opening the link and pressing Start in another app/tab —
// nothing on this page observes that happening directly). Off by default
// (refetchInterval: false) since polling makes no sense once the status is
// already settled and the user isn't in the middle of connecting.
const pollIntervalWhileConnectingMs = 3000;

export function useTelegramLinkStatus(pollWhileConnecting = false) {
  return useQuery({
    queryKey: telegramKeys.status(),
    queryFn: async (): Promise<TelegramStatus> => {
      const response = await getTelegramLinkStatus({ client: customApiClient });
      if (response.response?.status === 503) {
        return { configured: false, linked: false };
      }
      const data = handleResponse<GetTelegramLinkStatusResponse>(response);
      return { configured: true, linked: data.linked, linkedAt: data.linkedAt };
    },
    // A function (not a fixed interval) so polling stops itself the moment
    // a poll actually observes `linked: true`, without the caller having
    // to watch the result and toggle `pollWhileConnecting` back off.
    refetchInterval: (query) => {
      if (!pollWhileConnecting) return false;
      return query.state.data?.linked ? false : pollIntervalWhileConnectingMs;
    },
  });
}

// useCreateTelegramLinkCode is a mutation, not a query: every call issues
// a fresh, single-use, short-lived code (see the API's own doc comment),
// so this has a real side effect each time it runs rather than being
// something to cache/refetch automatically.
export function useCreateTelegramLinkCode() {
  return useMutation({
    mutationFn: async () => {
      const response = await getTelegramLinkCode({ client: customApiClient });
      return handleResponse<GetTelegramLinkCodeResponse>(response);
    },
  });
}

export function useUnlinkTelegramChat() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const response = await unlinkTelegramChat({ client: customApiClient });
      handleResponse(response);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: telegramKeys.status() });
    },
  });
}
