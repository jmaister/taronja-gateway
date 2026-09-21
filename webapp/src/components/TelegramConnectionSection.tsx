import { useState } from 'react';
import { useTelegramLinkStatus, useCreateTelegramLinkCode, useUnlinkTelegramChat } from '../services/telegram';
import { Button } from './ui/Button';
import { Card, CardContent } from './ui/Card';
import { ConfirmDialog } from './ui/ConfirmDialog';
import { StatusPill } from './ui/StatusPill';

// TelegramConnectionSection lets the current user connect their account to
// a Telegram chat (so notifications can be delivered there too) and see/
// change that connection's status — the profile-page counterpart to
// db.NotificationChannelLink, which is otherwise only ever read internally
// when a notification is actually delivered.
export const TelegramConnectionSection = () => {
    const [connecting, setConnecting] = useState(false);
    const [confirmingDisconnect, setConfirmingDisconnect] = useState(false);

    // Polls while `connecting` is true — nothing on this page observes the
    // user pressing "Start" in Telegram directly, so this is how a
    // successful connection is actually noticed. useTelegramLinkStatus
    // stops the polling itself once a poll reports linked (see its own
    // comment), and the render below already switches to the "Connected"
    // view on its own the moment that happens (that branch is checked
    // first, regardless of `connecting`'s stale value) — so no
    // effect+setState is needed here to react to the query settling.
    const { data: status, isLoading, error: statusError } = useTelegramLinkStatus(connecting);
    const createLinkCode = useCreateTelegramLinkCode();
    const unlink = useUnlinkTelegramChat();

    // Nothing to show at all when this gateway has no Telegram delivery
    // configured — there's no feature here to offer, not an error to
    // surface on someone's own profile page.
    if (!isLoading && status && !status.configured) {
        return null;
    }

    const handleConnect = async () => {
        setConnecting(true);
        try {
            await createLinkCode.mutateAsync();
        } catch {
            setConnecting(false);
        }
    };

    const handleCancelConnect = () => {
        setConnecting(false);
        createLinkCode.reset();
    };

    const confirmDisconnect = async () => {
        try {
            await unlink.mutateAsync();
            setConfirmingDisconnect(false);
            // Reset the "waiting to connect" state too — without this, a
            // stale connecting=true from a previous connect would make a
            // later reconnect attempt incorrectly render the old
            // (expired) deep link's "waiting" prompt instead of the
            // "Connect via Telegram" button.
            setConnecting(false);
            createLinkCode.reset();
        } catch {
            // Leave the dialog open — the error banner below (wired to
            // unlink.error) explains what happened, same pattern as
            // UserTokensSection's revoke confirmation.
        }
    };

    const errorMessage = (() => {
        const err = statusError || createLinkCode.error || unlink.error;
        if (!err) return null;
        return err instanceof Error ? err.message : String(err);
    })();

    return (
        <div className="mt-8">
            <div className="mb-6 flex flex-wrap items-center justify-between gap-4 border-b border-border pb-3">
                <div>
                    <h2 className="text-xl font-semibold">Telegram Notifications</h2>
                    <p className="mt-1 text-sm text-muted-fg">Connect your Telegram account to receive notifications there too.</p>
                </div>
            </div>

            {errorMessage && (
                <div className="mb-4 rounded-lg border border-danger/30 bg-danger/5 px-4 py-3 text-danger" role="alert">
                    {errorMessage}
                </div>
            )}

            {isLoading ? (
                <div className="py-4 text-center">
                    <div className="inline-block h-6 w-6 animate-spin rounded-full border-b-2 border-primary"></div>
                </div>
            ) : (
                <Card>
                    <CardContent className="flex flex-wrap items-center justify-between gap-4 py-5">
                        {status?.linked ? (
                            <>
                                <div>
                                    <StatusPill variant="success">Connected</StatusPill>
                                    {status.linkedAt && (
                                        <p className="mt-2 text-sm text-muted-fg">
                                            Since {new Date(status.linkedAt).toLocaleString()}
                                        </p>
                                    )}
                                </div>
                                <Button
                                    variant="danger"
                                    size="sm"
                                    onClick={() => setConfirmingDisconnect(true)}
                                    disabled={unlink.isPending}
                                >
                                    Disconnect
                                </Button>
                            </>
                        ) : connecting && createLinkCode.data ? (
                            <div className="w-full space-y-3">
                                <StatusPill variant="info">Waiting for you to connect…</StatusPill>
                                <p className="text-sm text-muted-fg">
                                    Open the link below and press Start in Telegram. This page updates automatically once you're
                                    connected.
                                </p>
                                <div className="flex flex-wrap items-center gap-3">
                                    <a
                                        href={createLinkCode.data.deepLink}
                                        target="_blank"
                                        rel="noreferrer"
                                        className="inline-flex h-10 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-fg hover:bg-primary/90"
                                    >
                                        Open in Telegram
                                    </a>
                                    <Button variant="secondary" size="sm" onClick={handleCancelConnect}>
                                        Cancel
                                    </Button>
                                </div>
                                <p className="text-xs text-muted-fg">
                                    Link expires {new Date(createLinkCode.data.expiresAt).toLocaleTimeString()}.
                                </p>
                            </div>
                        ) : (
                            <>
                                <StatusPill>Not connected</StatusPill>
                                <Button size="sm" onClick={() => void handleConnect()} disabled={createLinkCode.isPending}>
                                    {createLinkCode.isPending ? 'Connecting…' : 'Connect via Telegram'}
                                </Button>
                            </>
                        )}
                    </CardContent>
                </Card>
            )}

            <ConfirmDialog
                open={confirmingDisconnect}
                title="Disconnect Telegram?"
                description="You'll stop receiving notifications in Telegram until you connect again."
                confirmLabel={unlink.isPending ? 'Disconnecting…' : 'Disconnect'}
                variant="danger"
                isLoading={unlink.isPending}
                onCancel={() => setConfirmingDisconnect(false)}
                onConfirm={() => void confirmDisconnect()}
            />
        </div>
    );
};
