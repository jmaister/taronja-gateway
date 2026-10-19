import { Fragment, useState } from 'react';
import type { FormEvent } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { AdminNotification } from '@/apiclient/types.gen';
import { QueryState } from '../components/QueryState';
import { Card, CardContent } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import { StatusPill } from '../components/ui/StatusPill';
import { formatDateTime } from '../lib/format';
import { useAdminNotifications, useNotificationDeliveries, useNotificationTypes } from '../services/adminNotifications';
import { readFilters, toQuery, type FilterKey, type Filters } from '../lib/notificationFilters';

const CHANNELS = ['email', 'telegram', 'sms', 'whatsapp', 'response_webhook'];
const STATUSES = ['sent', 'pending', 'failed', 'skipped', 'none'] as const;
const PAGE_SIZES = [25, 50, 100, 200];
const DEFAULT_PAGE_SIZE = 50;

function statusVariant(status: string): 'success' | 'warning' | 'danger' | 'default' | 'info' {
    switch (status) {
        case 'sent':
            return 'success';
        case 'pending':
            return 'info';
        case 'failed':
            return 'danger';
        case 'skipped':
            return 'warning';
        default:
            return 'default';
    }
}

function statusLabel(status: string): string {
    return status === 'none' ? 'in-app only' : status;
}

function FilterBar({ filters, onApply }: { filters: Filters; onApply: (filters: Filters) => void }) {
    const [draft, setDraft] = useState<Filters>(filters);
    const types = useNotificationTypes();
    const set = (key: FilterKey) => (e: { target: { value: string } }) => setDraft({ ...draft, [key]: e.target.value });
    const submit = (e: FormEvent) => {
        e.preventDefault();
        onApply(draft);
    };
    const clear = () => {
        const empty = readFilters(new URLSearchParams());
        setDraft(empty);
        onApply(empty);
    };
    const selectClass = 'tg-input';

    return (
        <Card>
            <CardContent>
                <form onSubmit={submit} className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                    <FormField label="From" htmlFor="nf-from">
                        <Input id="nf-from" type="date" value={draft.from} onChange={set('from')} />
                    </FormField>
                    <FormField label="To" htmlFor="nf-to">
                        <Input id="nf-to" type="date" value={draft.to} onChange={set('to')} />
                    </FormField>
                    <FormField label="Type" htmlFor="nf-type">
                        <select id="nf-type" className={selectClass} value={draft.type} onChange={set('type')}>
                            <option value="">Any type</option>
                            {(types.data ?? []).map(type => (
                                <option key={type} value={type}>{type}</option>
                            ))}
                        </select>
                    </FormField>
                    <FormField label="Recipient" htmlFor="nf-user" description="Part of a username or email">
                        <Input id="nf-user" value={draft.user} onChange={set('user')} placeholder="ana@example.com" />
                    </FormField>
                    <FormField label="Channel" htmlFor="nf-channel">
                        <select id="nf-channel" className={selectClass} value={draft.channel} onChange={set('channel')}>
                            <option value="">Any channel</option>
                            {CHANNELS.map(channel => (
                                <option key={channel} value={channel}>{channel}</option>
                            ))}
                        </select>
                    </FormField>
                    <FormField label="Status" htmlFor="nf-status" description={draft.channel ? `On ${draft.channel}` : 'Overall'}>
                        <select id="nf-status" className={selectClass} value={draft.status} onChange={set('status')}>
                            <option value="">Any status</option>
                            {STATUSES.map(status => (
                                <option key={status} value={status}>{statusLabel(status)}</option>
                            ))}
                        </select>
                    </FormField>
                    <FormField label="Answered" htmlFor="nf-responded">
                        <select id="nf-responded" className={selectClass} value={draft.responded} onChange={set('responded')}>
                            <option value="">Either</option>
                            <option value="yes">Answered</option>
                            <option value="no">Not answered</option>
                        </select>
                    </FormField>
                    <FormField label="Text" htmlFor="nf-q" description="In the title or body">
                        <Input id="nf-q" value={draft.q} onChange={set('q')} />
                    </FormField>
                    {draft.batch && (
                        <FormField label="Batch" htmlFor="nf-batch">
                            <Input id="nf-batch" value={draft.batch} onChange={set('batch')} />
                        </FormField>
                    )}
                    <div className="flex items-end gap-3 sm:col-span-2 lg:col-span-4">
                        <Button type="submit">Apply filters</Button>
                        <Button type="button" variant="secondary" onClick={clear}>Clear</Button>
                    </div>
                </form>
            </CardContent>
        </Card>
    );
}

function DeliveryHistory({ notificationId }: { notificationId: string }) {
    const deliveries = useNotificationDeliveries(notificationId, true);
    if (deliveries.isLoading) return <p className="text-sm text-muted-fg">Loading delivery attempts…</p>;
    if (deliveries.error) return <p className="text-sm text-danger">{deliveries.error.message}</p>;
    if (!deliveries.data?.length) return <p className="text-sm text-muted-fg">No external delivery: in-app only.</p>;
    return (
        <table className="w-full text-xs">
            <thead>
                <tr className="text-left text-muted-fg">
                    <th className="py-1 pr-4 font-medium">When</th>
                    <th className="py-1 pr-4 font-medium">Channel</th>
                    <th className="py-1 pr-4 font-medium">Attempt</th>
                    <th className="py-1 pr-4 font-medium">Status</th>
                    <th className="py-1 font-medium">Detail</th>
                </tr>
            </thead>
            <tbody>
                {deliveries.data.map(d => (
                    <tr key={d.id} className="align-top">
                        <td className="py-1 pr-4 whitespace-nowrap">{formatDateTime(d.createdAt)}</td>
                        <td className="py-1 pr-4">{d.channel}</td>
                        <td className="py-1 pr-4">{d.attemptNumber}</td>
                        <td className="py-1 pr-4">
                            <StatusPill variant={statusVariant(d.status)}>{d.status}</StatusPill>
                        </td>
                        <td className="py-1 break-all">
                            {d.error || (d.nextRetryAt ? `Retry at ${formatDateTime(d.nextRetryAt)}` : '')}
                        </td>
                    </tr>
                ))}
            </tbody>
        </table>
    );
}

function NotificationRow({ item, open, onToggle, onFilterBatch }: {
    item: AdminNotification;
    open: boolean;
    onToggle: () => void;
    onFilterBatch: (batchId: string) => void;
}) {
    const n = item.notification;
    const channels = Object.entries(item.channels).sort(([a], [b]) => a.localeCompare(b));
    return (
        <Fragment>
            <tr className="border-b border-border hover:bg-muted/30 cursor-pointer" onClick={onToggle} aria-expanded={open}>
                <td className="px-4 py-3 text-sm whitespace-nowrap">{formatDateTime(n.createdAt)}</td>
                <td className="px-4 py-3 text-sm">
                    <div>{item.username || <span className="text-muted-fg">deleted user</span>}</div>
                    <div className="text-xs text-muted-fg">{item.email}</div>
                </td>
                <td className="px-4 py-3 text-sm font-mono">{n.type}</td>
                <td className="px-4 py-3 text-sm">{n.title}</td>
                <td className="px-4 py-3 text-sm">
                    <div className="flex flex-wrap gap-1">
                        {channels.length === 0 && <span className="text-muted-fg">—</span>}
                        {channels.map(([channel, status]) => (
                            <StatusPill key={channel} variant={statusVariant(status)} title={`${channel}: ${status}`}>
                                {channel}
                            </StatusPill>
                        ))}
                    </div>
                </td>
                <td className="px-4 py-3 text-sm">
                    <StatusPill variant={statusVariant(item.status)}>{statusLabel(item.status)}</StatusPill>
                </td>
                <td className="px-4 py-3 text-sm">
                    {n.respondedActionId ? (
                        <span title={n.respondedAt ? formatDateTime(n.respondedAt) : undefined}>
                            {n.respondedActionId}
                            {n.respondedVia && <span className="text-xs text-muted-fg"> via {n.respondedVia}</span>}
                        </span>
                    ) : (
                        <span className="text-muted-fg">—</span>
                    )}
                </td>
            </tr>
            {open && (
                <tr className="border-b border-border bg-muted/20">
                    <td colSpan={7} className="px-4 py-4 space-y-3">
                        <p className="text-sm whitespace-pre-line">{n.body}</p>
                        <div className="flex flex-wrap gap-4 text-xs text-muted-fg">
                            {n.url && <a className="text-primary underline" href={n.url} target="_blank" rel="noreferrer">{n.url}</a>}
                            <span>Read: {n.readAt ? formatDateTime(n.readAt) : 'no'}</span>
                            <button type="button" className="text-primary underline" onClick={() => onFilterBatch(n.batchId)}>
                                Show batch {n.batchId}
                            </button>
                        </div>
                        <DeliveryHistory notificationId={n.id} />
                    </td>
                </tr>
            )}
        </Fragment>
    );
}

export function NotificationsPage() {
    const [searchParams, setSearchParams] = useSearchParams();
    const filters = readFilters(searchParams);
    const page = Math.max(1, Number(searchParams.get('page')) || 1);
    const limit = PAGE_SIZES.includes(Number(searchParams.get('limit'))) ? Number(searchParams.get('limit')) : DEFAULT_PAGE_SIZE;
    const list = useAdminNotifications(toQuery(filters, page, limit));
    const [openId, setOpenId] = useState<string | null>(null);

    const update = (changes: Partial<Record<FilterKey | 'page' | 'limit', string>>) => {
        const next = new URLSearchParams(searchParams);
        for (const [key, value] of Object.entries(changes)) {
            if (value) next.set(key, value);
            else next.delete(key);
        }
        setSearchParams(next, { replace: true });
    };
    // A new filter starts again from the first page.
    const applyFilters = (next: Filters) => update({ ...next, page: '' });

    const total = list.data?.total ?? 0;
    const pages = Math.max(1, Math.ceil(total / limit));
    const first = total === 0 ? 0 : (page - 1) * limit + 1;
    const last = Math.min(page * limit, total);

    return (
        <div className="mx-auto max-w-7xl space-y-6">
            <PageHeader
                title="Notifications"
                description="Every notification sent through the gateway, with its delivery status per channel. Open a row for its delivery attempts and errors."
            />

            {/* Remounted when the URL filters change, so the form shows them. */}
            <FilterBar key={searchParams.toString()} filters={filters} onApply={applyFilters} />

            <QueryState isLoading={list.isLoading} error={list.error} onRetry={() => list.refetch()}>
                <Card>
                    <div className="overflow-x-auto">
                        <table className="w-full">
                            <thead>
                                <tr className="border-b border-border text-left text-sm text-muted-fg">
                                    <th className="px-4 py-3 font-medium">Created</th>
                                    <th className="px-4 py-3 font-medium">Recipient</th>
                                    <th className="px-4 py-3 font-medium">Type</th>
                                    <th className="px-4 py-3 font-medium">Title</th>
                                    <th className="px-4 py-3 font-medium">Channels</th>
                                    <th className="px-4 py-3 font-medium">Status</th>
                                    <th className="px-4 py-3 font-medium">Answer</th>
                                </tr>
                            </thead>
                            <tbody>
                                {list.data?.items.length === 0 && (
                                    <tr>
                                        <td colSpan={7} className="px-4 py-8 text-center text-sm text-muted-fg">
                                            No notifications match these filters.
                                        </td>
                                    </tr>
                                )}
                                {list.data?.items.map(item => (
                                    <NotificationRow
                                        key={item.notification.id}
                                        item={item}
                                        open={openId === item.notification.id}
                                        onToggle={() => setOpenId(openId === item.notification.id ? null : item.notification.id)}
                                        onFilterBatch={batch => applyFilters({ ...readFilters(new URLSearchParams()), batch })}
                                    />
                                ))}
                            </tbody>
                        </table>
                    </div>
                    <CardContent className="flex flex-wrap items-center justify-between gap-3 border-t border-border text-sm">
                        <span className="text-muted-fg">
                            {total === 0 ? 'No results' : `Showing ${first}–${last} of ${total}`}
                        </span>
                        <div className="flex items-center gap-3">
                            <label className="flex items-center gap-2 text-muted-fg">
                                Per page
                                <select
                                    className="tg-input w-auto"
                                    value={limit}
                                    onChange={e => update({ limit: e.target.value, page: '' })}
                                >
                                    {PAGE_SIZES.map(size => (
                                        <option key={size} value={size}>{size}</option>
                                    ))}
                                </select>
                            </label>
                            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => update({ page: String(page - 1) })}>
                                Previous
                            </Button>
                            <span>Page {page} of {pages}</span>
                            <Button variant="outline" size="sm" disabled={page >= pages} onClick={() => update({ page: String(page + 1) })}>
                                Next
                            </Button>
                        </div>
                    </CardContent>
                </Card>
            </QueryState>
        </div>
    );
}
