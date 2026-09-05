import { useState } from 'react';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { useMiddlewareStatus, useMiddlewareMetrics } from '../services/services';
import type { MiddlewareStatusItem, MiddlewareMetrics } from '@/apiclient/types.gen';

function healthBadgeVariant(status?: string): 'success' | 'warning' | 'danger' | 'default' {
    switch (status) {
        case 'healthy':
            return 'success';
        case 'degraded':
            return 'warning';
        case 'unhealthy':
            return 'danger';
        default:
            return 'default';
    }
}

function formatDuration(ms: number): string {
    if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`;
    return `${ms.toFixed(2)}ms`;
}

function formatLastInvoked(ts?: string | null): string {
    if (!ts) return 'never';
    return new Date(ts).toLocaleTimeString();
}

function MiddlewareRow({ item, metrics }: { item: MiddlewareStatusItem; metrics?: MiddlewareMetrics }) {
    return (
        <tr className="border-b border-border last:border-0 hover:bg-muted/30 transition-colors">
            <td className="px-4 py-3">
                <div className="font-mono text-sm">{item.name}</div>
                <div className="text-xs text-muted-fg mt-0.5">{item.description}</div>
            </td>
            <td className="px-4 py-3 text-center">
                <Badge variant={item.status === 'active' ? 'success' : 'default'}>{item.status}</Badge>
            </td>
            <td className="px-4 py-3 text-center">
                <Badge variant={healthBadgeVariant(item.health?.status)}>{item.health?.status ?? 'unknown'}</Badge>
                {item.health?.message && (
                    <div className="text-xs text-muted-fg mt-1">{item.health.message}</div>
                )}
            </td>
            <td className="px-4 py-3 text-sm">
                {item.dependencies && item.dependencies.length > 0 ? (
                    <span className="font-mono text-xs">{item.dependencies.join(', ')}</span>
                ) : (
                    <span className="text-muted-fg">—</span>
                )}
            </td>
            <td className="px-4 py-3 text-center text-sm">{metrics ? metrics.requestCount : '—'}</td>
            <td className="px-4 py-3 text-center text-sm">{metrics ? metrics.errorCount : '—'}</td>
            <td className="px-4 py-3 text-center text-sm">{metrics ? formatDuration(metrics.averageDurationMs) : '—'}</td>
            <td className="px-4 py-3 text-center text-sm text-muted-fg">{metrics ? formatLastInvoked(metrics.lastInvokedAt) : '—'}</td>
        </tr>
    );
}

export function MiddlewarePage() {
    const [autoRefresh, setAutoRefresh] = useState(true);
    const status = useMiddlewareStatus();
    const metrics = useMiddlewareMetrics();

    // Auto-refresh: re-fetch every 10 seconds when enabled, same pattern as RateLimiterStatsPage.
    useState(() => {
        if (!autoRefresh) return;
        const id = setInterval(() => {
            status.refetch();
            metrics.refetch();
        }, 10_000);
        return () => clearInterval(id);
    });

    const isLoading = status.isLoading || metrics.isLoading;
    const error = status.error ?? metrics.error;

    const metricsByName = new Map((metrics.data ?? []).map(m => [m.name, m]));
    const items = status.data ?? [];
    const activeCount = items.filter(i => i.status === 'active').length;
    const unhealthyCount = items.filter(i => i.health?.status === 'unhealthy' || i.health?.status === 'degraded').length;

    return (
        <div className="mx-auto max-w-7xl space-y-6">
            {/* Header */}
            <div className="flex items-center justify-between">
                <div>
                    <h1 className="text-2xl font-semibold tracking-tight">Middleware</h1>
                    <p className="text-sm text-muted-fg mt-1">
                        Status, health, and live request metrics for the global middleware chain.
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
                        onClick={() => { status.refetch(); metrics.refetch(); }}
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
                        <div className="text-3xl font-semibold text-primary">{items.length}</div>
                        <div className="text-sm text-muted-fg mt-1">Middleware registered</div>
                    </CardContent>
                </Card>
                <Card>
                    <CardContent className="pt-5">
                        <div className="text-3xl font-semibold text-success">{activeCount}</div>
                        <div className="text-sm text-muted-fg mt-1">Active in the chain</div>
                    </CardContent>
                </Card>
                <Card>
                    <CardContent className="pt-5">
                        <div className={`text-3xl font-semibold ${unhealthyCount > 0 ? 'text-danger' : 'text-fg'}`}>
                            {unhealthyCount}
                        </div>
                        <div className="text-sm text-muted-fg mt-1">Degraded or unhealthy</div>
                    </CardContent>
                </Card>
            </div>

            {/* Middleware Table */}
            <Card>
                <CardHeader>
                    <h3 className="text-base font-semibold">Global Middleware Chain</h3>
                </CardHeader>
                <CardContent className="p-0">
                    {isLoading ? (
                        <div className="p-6 space-y-3">
                            {[...Array(4)].map((_, i) => (
                                <div key={i} className="animate-pulse h-8 rounded bg-muted"></div>
                            ))}
                        </div>
                    ) : error ? (
                        <div className="p-6 text-sm text-danger" role="alert">
                            Failed to load middleware status. Make sure you're signed in as an admin.
                        </div>
                    ) : items.length === 0 ? (
                        <div className="p-6 text-sm text-muted-fg">
                            No middleware registered.
                        </div>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-left">
                                <thead>
                                    <tr className="border-b border-border bg-muted/40">
                                        <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-fg">Middleware</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Status</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Health</th>
                                        <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-fg">Depends on</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Requests</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Errors</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Avg Duration</th>
                                        <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-muted-fg">Last Invoked</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {items.map(item => (
                                        <MiddlewareRow key={item.name} item={item} metrics={metricsByName.get(item.name)} />
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </CardContent>
            </Card>
        </div>
    );
}
