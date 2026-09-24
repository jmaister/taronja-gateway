import { useUserChannelStatuses, useUserNotificationPreference } from '../services/channelStatus';
import { Card, CardContent, CardHeader } from './ui/Card';
import { ChannelStatusList } from './ChannelStatusList';

interface UserNotificationsSectionProps {
    userId: string;
}

/**
 * UserNotificationsSection is the admin-facing counterpart to
 * ConnectionStatusPage/ProfilePage's Telegram section: on a specific
 * user's detail page, it shows their preferred notification channel and
 * their status on every channel this gateway has configured (e.g.
 * whether they've connected Telegram), read-only — an admin can see this
 * to understand why a user is or isn't receiving notifications, without
 * being able to change it on their behalf.
 */
export const UserNotificationsSection = ({ userId }: UserNotificationsSectionProps) => {
    const { data: statuses, isLoading: statusesLoading, error: statusesError } = useUserChannelStatuses(userId);
    const { data: preference, isLoading: preferenceLoading, error: preferenceError } = useUserNotificationPreference(userId);

    const isLoading = statusesLoading || preferenceLoading;
    const error = statusesError ?? preferenceError;

    return (
        <div className="mt-8">
            <div className="mb-4 border-b border-border pb-2">
                <h2 className="text-base font-semibold">Notifications</h2>
            </div>

            {error && (
                <div className="mb-4 rounded-lg border border-danger/30 bg-danger/5 px-4 py-3 text-danger" role="alert">
                    {error instanceof Error ? error.message : String(error)}
                </div>
            )}

            {isLoading ? (
                <div className="py-4 text-center">
                    <div className="inline-block h-6 w-6 animate-spin rounded-full border-b-2 border-primary"></div>
                </div>
            ) : (
                <Card>
                    <CardHeader className="text-sm">
                        <span className="font-medium text-muted-fg">Preferred channel: </span>
                        {preference?.preferredChannel ? (
                            <span className="font-medium capitalize">{preference.preferredChannel}</span>
                        ) : (
                            <span className="text-muted-fg">None set — every configured channel is used</span>
                        )}
                    </CardHeader>
                    <CardContent className="p-0">
                        <ChannelStatusList statuses={statuses ?? []} />
                    </CardContent>
                </Card>
            )}
        </div>
    );
};
