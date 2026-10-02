import { StatusPill } from './ui/StatusPill';
import type { ChannelStatus } from '@/apiclient';

// statusPillVariant and statusLabel translate a ChannelStatus.status onto
// StatusPill's variant/label — "active" (a channel that works for every
// user already, e.g. email) reads the same as "connected" (a
// ConnectableProvider this user has linked, e.g. telegram): both mean
// this channel can reach this user right now. "not_connected" is the one
// state that actually needs attention.
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
 * ChannelStatusList renders one row per configured notification channel —
 * shared by ConnectionStatusPage (the current user's own status, every
 * channel) and UserNotificationsSection (an admin looking at one other
 * user's status, alongside their preferred channel).
 */
export function ChannelStatusList({ statuses }: { statuses: ChannelStatus[] }) {
    if (statuses.length === 0) {
        return <p className="px-5 py-8 text-center text-sm text-muted-fg">No notification channels are configured on this gateway.</p>;
    }

    return (
        <div className="divide-y divide-border">
            {statuses.map((cs) => (
                <div key={cs.channel} className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
                    <div>
                        <div className="font-medium capitalize">{cs.channel}</div>
                        {cs.status === 'connected' && cs.linkedAt && (
                            <div className="mt-1 text-sm text-muted-fg">Since {new Date(cs.linkedAt).toLocaleString()}</div>
                        )}
                    </div>
                    <StatusPill variant={statusPillVariant(cs.status)}>{statusLabel(cs.status)}</StatusPill>
                </div>
            ))}
        </div>
    );
}
