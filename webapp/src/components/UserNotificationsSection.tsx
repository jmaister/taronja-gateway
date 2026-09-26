import { useState } from 'react';
import { useUserChannelStatuses, useUserNotificationPreference, useGenerateUserTelegramLinkCode } from '../services/channelStatus';
import { Card, CardContent, CardHeader } from './ui/Card';
import { Button } from './ui/Button';
import { Input } from './ui/Input';
import { ChannelStatusList } from './ChannelStatusList';

interface UserNotificationsSectionProps {
    userId: string;
}

/**
 * UserNotificationsSection is the admin-facing counterpart to
 * ConnectionStatusPage/ProfilePage's Telegram section: on a specific
 * user's detail page, it shows their preferred notification channel and
 * their status on every channel this gateway has configured (e.g.
 * whether they've connected Telegram) — so an admin can see why a user is
 * or isn't receiving notifications. Unlike the self-service pages, this
 * doesn't let an admin change the preference or disconnect a channel on
 * the user's behalf; the one action it does offer is generating a
 * Telegram connect link when the user hasn't linked one yet, so an admin
 * can hand it to them directly (message, email, ...) rather than the user
 * finding their own way to their profile page's Connect button.
 */
export const UserNotificationsSection = ({ userId }: UserNotificationsSectionProps) => {
    const { data: statuses, isLoading: statusesLoading, error: statusesError } = useUserChannelStatuses(userId);
    const { data: preference, isLoading: preferenceLoading, error: preferenceError } = useUserNotificationPreference(userId);
    const generateLink = useGenerateUserTelegramLinkCode();
    const [copied, setCopied] = useState(false);

    const isLoading = statusesLoading || preferenceLoading;
    const error = statusesError ?? preferenceError ?? generateLink.error;

    const telegramStatus = statuses?.find((cs) => cs.channel === 'telegram');
    const canGenerateTelegramLink = telegramStatus?.status === 'not_connected';
    const linkCode = generateLink.data;

    const handleGenerateLink = async () => {
        setCopied(false);
        await generateLink.mutateAsync(userId);
    };

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

                    {canGenerateTelegramLink && (
                        <CardContent className="space-y-3 border-t border-border">
                            {linkCode ? (
                                <div className="space-y-2">
                                    <label className="block text-sm font-medium text-muted-fg">
                                        Telegram connect link — expires {new Date(linkCode.expiresAt).toLocaleTimeString()}
                                    </label>
                                    <div className="flex flex-wrap items-center gap-2">
                                        <Input
                                            type="text"
                                            value={linkCode.deepLink}
                                            readOnly
                                            className={`min-w-64 flex-1 font-mono text-sm ${copied ? 'border-success bg-success/10' : ''}`}
                                            onFocus={(e) => e.currentTarget.select()}
                                        />
                                        <Button
                                            variant="secondary"
                                            size="sm"
                                            onClick={async () => {
                                                await navigator.clipboard.writeText(linkCode.deepLink);
                                                setCopied(true);
                                                setTimeout(() => setCopied(false), 800);
                                            }}
                                        >
                                            {copied ? 'Copied!' : 'Copy link'}
                                        </Button>
                                        <Button variant="outline" size="sm" onClick={() => void handleGenerateLink()} disabled={generateLink.isPending}>
                                            Generate new link
                                        </Button>
                                    </div>
                                </div>
                            ) : (
                                <Button size="sm" onClick={() => void handleGenerateLink()} disabled={generateLink.isPending}>
                                    {generateLink.isPending ? 'Generating…' : 'Generate Telegram connect link'}
                                </Button>
                            )}
                        </CardContent>
                    )}
                </Card>
            )}
        </div>
    );
};
