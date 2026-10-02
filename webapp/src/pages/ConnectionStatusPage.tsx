import { useChannelStatuses } from '../services/channelStatus';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { ChannelStatusList } from '../components/ChannelStatusList';

/**
 * ConnectionStatusPage lists every notification channel this gateway has
 * configured and the current user's status on it — the generic
 * counterpart to ProfilePage's Telegram-specific connect/disconnect
 * section, useful once there's more than one channel that requires
 * per-user linking to see all of them at a glance in one place.
 */
export const ConnectionStatusPage = () => {
    const { data: statuses, isLoading, error, refetch, isFetching } = useChannelStatuses();

    return (
        <div className="mx-auto max-w-7xl space-y-6">
            <div className="flex items-center justify-between">
                <div>
                    <h1 className="text-2xl font-semibold tracking-tight">Notification Channels</h1>
                    <p className="mt-1 text-sm text-muted-fg">
                        Every channel this gateway has configured, and your own status on each.
                    </p>
                </div>
                <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
                    {isFetching ? 'Refreshing…' : 'Refresh'}
                </Button>
            </div>

            {error && (
                <div className="rounded-lg border border-danger/30 bg-danger/5 px-4 py-3 text-danger" role="alert">
                    {error instanceof Error ? error.message : String(error)}
                </div>
            )}

            {isLoading ? (
                <div className="py-8 text-center">
                    <div className="inline-block h-6 w-6 animate-spin rounded-full border-b-2 border-primary"></div>
                </div>
            ) : (
                <Card>
                    <CardHeader className="text-sm font-medium text-muted-fg">Channels</CardHeader>
                    <CardContent className="p-0">
                        <ChannelStatusList statuses={statuses ?? []} />
                    </CardContent>
                </Card>
            )}
        </div>
    );
};

export default ConnectionStatusPage;
