import { listAdminNotifications, listNotificationTypes, listNotificationDeliveries } from '@/apiclient/sdk.gen';
import type { AdminNotificationList, ListAdminNotificationsData, NotificationDelivery } from '@/apiclient/types.gen';
import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export type AdminNotificationQuery = NonNullable<ListAdminNotificationsData['query']>;

export const adminNotificationKeys = {
  list: (query: AdminNotificationQuery) => ['adminNotifications', query] as const,
  types: () => ['adminNotifications', 'types'] as const,
  deliveries: (notificationId: string) => ['adminNotifications', 'deliveries', notificationId] as const,
};

// Every user's notifications, filtered and paginated. The previous page stays
// on screen while the next one loads.
export function useAdminNotifications(query: AdminNotificationQuery) {
  return useQuery<AdminNotificationList, Error>({
    queryKey: adminNotificationKeys.list(query),
    queryFn: async () => {
      const response = await listAdminNotifications({ query, client: customApiClient });
      return handleResponse<AdminNotificationList>(response);
    },
    placeholderData: keepPreviousData,
    staleTime: 10_000,
  });
}

export function useNotificationTypes() {
  return useQuery<string[], Error>({
    queryKey: adminNotificationKeys.types(),
    queryFn: async () => {
      const response = await listNotificationTypes({ client: customApiClient });
      return handleResponse<{ types: string[] }>(response).types;
    },
    staleTime: 60_000,
  });
}

// One notification's delivery attempts, newest first; loaded when its row opens.
export function useNotificationDeliveries(notificationId: string, enabled: boolean) {
  return useQuery<NotificationDelivery[], Error>({
    queryKey: adminNotificationKeys.deliveries(notificationId),
    queryFn: async () => {
      const response = await listNotificationDeliveries({ path: { notificationId }, client: customApiClient });
      return handleResponse<{ deliveries: NotificationDelivery[] }>(response).deliveries;
    },
    enabled,
  });
}
