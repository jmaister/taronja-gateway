import { useChannelStatuses } from '../services/channelStatus';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { StatusPill } from '../components/ui/StatusPill';
import type { ChannelStatus } from '@/apiclient';

// statusPillVariant and statusLabel translate a ChannelStatus.status onto
// StatusPill's variant/label — "active" (a channel that works for every
// user already, e.g. email) reads the same as "connected" (a
// ConnectableProvider this user has linked, e.g. telegram): both mean
// this channel can reach this user right now. "not_connected" is the one
// state that actually needs the user's attention.
function statusPillVariant(status: ChannelStatus['status']): 'success' | 'warning' {
    return status === 'not_connected' ? 'warning' : 'success';
}

function statusLabel(status: ChannelStatus['status']): string {
    switch (status) {
        case 'active':
            return 'Active';
        case 'connected':
            return 'Connected';
        case 'not_connected':
            return 'Not connected';
    }
}

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
            ) : !statuses || statuses.length === 0 ? (
                <Card>
                    <CardContent className="py-8 text-center text-sm text-muted-fg">
                        No notification channels are configured on this gateway.
                    </CardContent>
                </Card>
            ) : (
                <Card>
                    <CardHeader className="text-sm font-medium text-muted-fg">Channels</CardHeader>
                    <CardContent className="divide-y divide-border p-0">
                        {statuses.map((cs) => (
                            <div key={cs.channel} className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
                                <div>
                                    <div className="font-medium capitalize">{cs.channel}</div>
                                    {cs.status === 'connected' && cs.linkedAt && (
                                        <div className="mt-1 text-sm text-muted-fg">
                                            Since {new Date(cs.linkedAt).toLocaleString()}
                                        </div>
                                    )}
                                </div>
                                <StatusPill variant={statusPillVariant(cs.status)}>{statusLabel(cs.status)}</StatusPill>
                            </div>
                        ))}
                    </CardContent>
                </Card>
            )}
        </div>
    );
};

export default ConnectionStatusPage;
