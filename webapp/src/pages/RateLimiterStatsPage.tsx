import { InlineQueryState } from '../components/QueryState';
import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Button } from '../components/ui/Button';
import { FilterToggle } from '../components/FilterToggle';
import { toggleValue } from '../lib/toggleValue';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { useRateLimiterStats, useBlockedClients } from '../services/rateLimiter';
import type { RateLimiterStat, BlockedClient } from '../apiclient/types.gen';
import { LazyBlockedClientsWorldMap } from '../components/LazyBlockedClientsWorldMap';
import { formatDateTime } from '../lib/format';

function formatBlockedUntil(ts: string): { label: string; isBlocked: boolean } {
    const d = new Date(ts);
    const now = new Date();
    if (d <= now) {
        return { label: 'Not blocked', isBlocked: false };
    }
    const diffMs = d.getTime() - now.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffSec = Math.floor((diffMs % 60000) / 1000);
    const label = diffMin > 0 ? `${diffMin}m ${diffSec}s` : `${diffSec}s`;
    return { label: `Blocked (${label})`, isBlocked: true };
}

function StatRow({ stat }: { stat: RateLimiterStat }) {
    const blocked = formatBlockedUntil(stat.blockedUntil);
    return (
        <tr className="border-b border-border last:border-0 hover:bg-muted/30 transition-colors">
            <td className="px-4 py-3 font-mono text-sm">{stat.ip}</td>
            <td className="px-4 py-3 text-center text-sm">{stat.requests}</td>
            <td className="px-4 py-3 text-center text-sm">{stat.errors}</td>
            <td className="px-4 py-3 text-center text-sm">{stat.scan404}</td>
            <td className="px-4 py-3 text-center">
                <Badge variant={blocked.isBlocked ? 'danger' : 'success'}>
                    {blocked.label}
                </Badge>
            </td>
        </tr>
    );
}

function reasonLabel(reason: BlockedClient['reason']): string {
    switch (reason) {
        case 'rate_limit':
            return 'Rate limit';
        case 'max_errors':
            return 'Too many errors';
        case 'vulnerability_scan':
            return 'Vulnerability scan';
        default:
            return reason;
    }
}

function BlockedClientRow({
    item,
    activeCountry,
    onCountrySelect,
}: {
    item: BlockedClient;
    activeCountry: string;
    onCountrySelect: (country: string) => void;
}) {
    const country = item.country;
    return (
        <tr className="border-b border-border last:border-0 hover:bg-muted/30 transition-colors">
            <td className="px-4 py-3 font-mono text-sm">{item.ipAddress}</td>
            <td className="px-4 py-3 text-sm">
                {country ? (
                    <FilterToggle
                        label={country}
                        active={country === activeCountry}
                        onToggle={() => onCountrySelect(country)}
                        className="px-1"
                    >
                        {country}
                    </FilterToggle>
                ) : (
                    <span className="text-muted-fg">—</span>
                )}
            </td>
            <td className="px-4 py-3 text-center">
                <Badge variant="danger">{reasonLabel(item.reason)}</Badge>
            </td>
            <td className="px-4 py-3 text-sm text-muted-fg">{item.path ?? '—'}</td>
            <td className="px-4 py-3 text-center text-sm">{item.triggerCount}</td>
            <td className="px-4 py-3 text-sm">{formatDateTime(item.blockedAt)}</td>
            <td className="px-4 py-3 text-sm">{formatDateTime(item.blockedUntil)}</td>
        </tr>
    );
}

export function RateLimiterStatsPage() {
    const [autoRefresh, setAutoRefresh] = useState(true);
    const { data: stats, isLoading, error, refetch, dataUpdatedAt } = useRateLimiterStats();
    const [searchParams, setSearchParams] = useSearchParams();
    const country = searchParams.get('country') ?? '';
    const selectCountry = (value: string) => {
        const next = new URLSearchParams(searchParams);
        const selected = toggleValue(country, value);
        if (selected) next.set('country', selected);
        else next.delete('country');
        setSearchParams(next, { replace: true });
    };
    // 200 is the API's max limit. The map always shows every country so
    // another one can be picked; the history table follows the country filter
    // (the same query as the map when no country is selected).
    const {
        data: blockedClients,
        isLoading: blockedClientsLoading,
        error: blockedClientsError,
    } = useBlockedClients({}, 200);
    const {
        data: history,
        isLoading: historyLoading,
        error: historyError,
    } = useBlockedClients(country ? { country } : {}, 200);

    // Auto-refresh: re-fetch every 10 seconds when enabled
    useState(() => {
        if (!autoRefresh) return;
        const id = setInterval(() => refetch(), 10_000);
        return () => clearInterval(id);
    });

    const blockedCount = stats?.filter(s => new Date(s.blockedUntil) > new Date()).length ?? 0;
    const totalIPs = stats?.length ?? 0;

    return (
        <div className="mx-auto max-w-7xl space-y-6">
            {/* Header */}
            <div className="flex items-center justify-between">
                <div>
                    <h1 className="text-2xl font-semibold tracking-tight">Rate Limiter Stats</h1>
                    <p className="text-sm text-muted-fg mt-1">
                        Live per-IP tracking: requests, errors, and blocked status.
                    </p>
                </div>
                <div className="flex items-center gap-3">
                    <label className="flex items-center gap-2 text-sm text-muted-fg cursor-pointer select-none">
                        <input
                            type="checkbox"
                            checked={autoRefresh}
                            onChange={e => setAutoRefresh(e.target.checked)}
                            className="accent-primary"
                        />
                        Auto-refresh
                    </label>
                    <button
                        onClick={() => refetch()}
                        className="rounded-lg border border-border px-3 py-1.5 text-sm font-medium hover:bg-muted/70 transition-colors"
                    >
                        Refresh
                    </button>
                </div>
            </div>

            {/* Summary Cards */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <Card>
                    <CardContent className="pt-5">
                        <div className="text-3xl font-semibold text-primary">{totalIPs}</div>
                        <div className="text-sm text-muted-fg mt-1">Active IPs tracked</div>
                    </CardContent>
                </Card>
                <Card>
                    <CardContent className="pt-5">
                        <div className="text-3xl font-semibold text-danger">{blockedCount}</div>
                        <div className="text-sm text-muted-fg mt-1">Currently blocked</div>
                    </CardContent>
                </Card>
                <Card>
                    <CardContent className="pt-5">
                        <div className="text-3xl font-semibold text-fg">
                            {dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString() : '—'}
                        </div>
                        <div className="text-sm text-muted-fg mt-1">Last updated</div>
                    </CardContent>
                </Card>
            </div>

            {/* Stats Table */}
            <Card>
                <CardHeader>
                    <h3 className="text-base font-semibold">IP Statistics</h3>
                </CardHeader>
                <CardContent className="p-0">
                    <InlineQueryState isLoading={isLoading} error={error} errorMessage="Failed to load rate limiter stats. Make sure the rate limiter is enabled.">
                        {!stats || stats.length === 0 ? (
                        <div className="p-6 text-sm text-muted-fg">
                            No IPs are currently being tracked. Traffic will appear here once requests are received.
                        </div>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-left">
                                <thead>
                                    <tr className="border-b border-border bg-muted/40">
                                        <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-fg">IP Address</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Requests</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Errors</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Scan 404s</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Status</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {[...stats]
                                        .sort((a, b) => {
                                            // Sort blocked IPs first, then by request count descending
                                            const aBlocked = new Date(a.blockedUntil) > new Date();
                                            const bBlocked = new Date(b.blockedUntil) > new Date();
                                            if (aBlocked !== bBlocked) return aBlocked ? -1 : 1;
                                            return b.requests - a.requests;
                                        })
                                        .map(stat => (
                                            <StatRow key={stat.ip} stat={stat} />
                                        ))
                                    }
                                </tbody>
                            </table>
                        </div>
                        )}
                    </InlineQueryState>
                </CardContent>
            </Card>

            {/* Attacker Map */}
            {!blockedClientsLoading && !blockedClientsError && blockedClients && blockedClients.items.length > 0 && (
                <LazyBlockedClientsWorldMap
                    blockedClients={blockedClients.items}
                    activeCountry={country}
                    onCountrySelect={selectCountry}
                />
            )}

            {/* Blocked Clients History */}
            <Card>
                <CardHeader>
                    <div className="flex flex-wrap items-start justify-between gap-3">
                        <div>
                            <h3 className="text-base font-semibold">Blocked Clients History</h3>
                            <p className="text-sm text-muted-fg mt-1">
                                Persistent record of past block events. Unlike the live stats above, entries here
                                remain after the rate limiter forgets an IP. Click a country to filter by it.
                            </p>
                        </div>
                        {country && (
                            <Button variant="secondary" size="sm" onClick={() => selectCountry(country)}>
                                Country: {country} ✕
                            </Button>
                        )}
                    </div>
                </CardHeader>
                <CardContent className="p-0">
                    <InlineQueryState isLoading={historyLoading} error={historyError} errorMessage="Failed to load blocked clients history.">
                        {!history || history.items.length === 0 ? (
                        <div className="p-6 text-sm text-muted-fg">
                            {country ? `No blocked clients from ${country}.` : 'No blocked clients recorded yet.'}
                        </div>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-left">
                                <thead>
                                    <tr className="border-b border-border bg-muted/40">
                                        <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-fg">IP Address</th>
                                        <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-fg">Country</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Reason</th>
                                        <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-fg">Path</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Triggers</th>
                                        <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-fg">Blocked At</th>
                                        <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-fg">Blocked Until</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {history.items.map(item => (
                                        <BlockedClientRow key={item.id} item={item} activeCountry={country} onCountrySelect={selectCountry} />
                                    ))}
                                </tbody>
                            </table>
                        </div>
                        )}
                    </InlineQueryState>
                </CardContent>
            </Card>
        </div>
    );
}
