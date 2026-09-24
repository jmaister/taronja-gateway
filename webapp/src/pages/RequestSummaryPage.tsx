import { useState } from 'react';
import { StatisticsDateRange, timePeriods, DateRange } from '../components/StatisticsDateRange';
import { useRequestStatistics, useRequestTimeSeries } from '../services/statistics';
import { Button } from '../components/ui/Button';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { PageHeader } from '../components/ui/PageHeader';
import { TimeSeriesChart } from '../components/charts/TimeSeriesChart';
import type { TimeSeriesGranularity } from '@/apiclient';

interface StatCard {
    title: string;
    value: string | number;
    icon: string;
    accent: 'primary' | 'success' | 'warning' | 'danger';
}

function StatisticCard({ title, value, icon, accent }: StatCard) {
    const accentClasses: Record<StatCard['accent'], { border: string; text: string }> = {
        primary: { border: 'border-primary', text: 'text-primary' },
        success: { border: 'border-success', text: 'text-success' },
        warning: { border: 'border-warning', text: 'text-warning' },
        danger: { border: 'border-danger', text: 'text-danger' },
    };

    return (
        <Card className={`border-l-4 ${accentClasses[accent].border}`}>
            <CardContent className="py-5">
                <div className="flex items-center justify-between">
                    <div>
                        <p className="text-sm font-medium text-muted-fg">{title}</p>
                        <p className="text-2xl font-semibold">{value}</p>
                    </div>
                    <div className={`text-3xl ${accentClasses[accent].text}`}>{icon}</div>
                </div>
            </CardContent>
        </Card>
    );
}

interface DataTableProps {
    title: string;
    data: Record<string, number>;
    accent?: 'primary' | 'success' | 'warning' | 'danger';
}

function DataTable({ title, data, accent = 'primary' }: DataTableProps) {
    const accentText: Record<NonNullable<DataTableProps['accent']>, string> = {
        primary: 'text-primary',
        success: 'text-success',
        warning: 'text-warning',
        danger: 'text-danger',
    };

    const accentDot: Record<NonNullable<DataTableProps['accent']>, string> = {
        primary: 'bg-primary',
        success: 'bg-success',
        warning: 'bg-warning',
        danger: 'bg-danger',
    };

    const sortedData = Object.entries(data)
        .filter(([key]) => key && key.trim() !== '')
        .sort(([, a], [, b]) => b - a)
        .slice(0, 10);

    if (sortedData.length === 0) {
        return (
            <Card>
                <CardHeader>
                    <h3 className={`text-base font-semibold ${accentText[accent]}`}>{title}</h3>
                </CardHeader>
                <CardContent>
                    <p className="py-8 text-center text-sm text-muted-fg">No data available</p>
                </CardContent>
            </Card>
        );
    }

    return (
        <Card>
            <CardHeader>
                <h3 className={`text-base font-semibold ${accentText[accent]}`}>{title}</h3>
            </CardHeader>
            <CardContent>
                <div className="space-y-2">
                    {sortedData.map(([key, value], index) => (
                        <div
                            key={key}
                            className="flex items-center justify-between border-b border-border/70 py-2 last:border-b-0"
                        >
                            <span className="text-sm font-medium text-muted-fg">{key}</span>
                            <div className="flex items-center space-x-2">
                                <span className="text-sm font-semibold">{value.toLocaleString()}</span>
                                <div
                                    className={`h-2 w-2 rounded-full ${accentDot[accent]}`}
                                    style={{ opacity: 1 - index * 0.1 }}
                                />
                            </div>
                        </div>
                    ))}
                </div>
            </CardContent>
        </Card>
    );
}

// granularityForRange picks the time-series bucket size that suits a
// date range's span, the same "get a sensible default for free" idea
// Grafana/Cloudflare/Vercel Analytics presets follow, but derived from
// whatever range the page's one date-range selector is set to rather
// than needing a second, independent selector of its own: an hour-wide
// bucket is fine to look at over a single day, but unreadable stretched
// across a year.
function granularityForRange(start: Date, end: Date): TimeSeriesGranularity {
    const spanDays = (end.getTime() - start.getTime()) / (24 * 60 * 60 * 1000);
    if (spanDays <= 1) return 'hour';
    if (spanDays <= 31) return 'day';
    if (spanDays <= 366) return 'week';
    return 'month';
}

function TrafficOverTimeSection({ start, end }: { start: Date; end: Date }) {
    const granularity = granularityForRange(start, end);
    const { data, isLoading, error } = useRequestTimeSeries(start.toISOString(), end.toISOString(), granularity);

    return (
        <Card>
            <CardHeader>
                <h3 className="text-base font-semibold">Traffic Over Time</h3>
            </CardHeader>
            <CardContent>
                {isLoading && <p className="py-8 text-center text-sm text-muted-fg">Loading…</p>}
                {error && (
                    <p className="py-8 text-center text-sm text-danger">
                        {error instanceof Error ? error.message : 'Failed to load traffic over time'}
                    </p>
                )}
                {data && (
                    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
                        <TimeSeriesChart
                            title="Requests & Unique Visitors"
                            points={data.points}
                            granularity={data.granularity}
                            lines={[
                                { key: 'requestCount', label: 'Requests', color: '#4299E1' },
                                { key: 'uniqueFingerprints', label: 'Unique Visitors (fingerprint)', color: '#48BB78' },
                                { key: 'uniqueUsers', label: 'Authenticated Users', color: '#9F7AEA' },
                            ]}
                        />
                        <TimeSeriesChart
                            title="New vs Returning Visitors"
                            points={data.points}
                            granularity={data.granularity}
                            lines={[
                                { key: 'newVisitors', label: 'New', color: '#48BB78' },
                                { key: 'returningVisitors', label: 'Returning', color: '#4299E1' },
                            ]}
                        />
                        <TimeSeriesChart
                            title="Errors Over Time"
                            points={data.points}
                            granularity={data.granularity}
                            lines={[{ key: 'errorCount', label: 'Errors (4xx/5xx)', color: '#F56565' }]}
                        />
                        <TimeSeriesChart
                            title="Average Response Time"
                            points={data.points}
                            granularity={data.granularity}
                            lines={[{ key: 'averageResponseTime', label: 'Avg Response Time', color: '#ED8936' }]}
                            unit="ms"
                        />
                    </div>
                )}
            </CardContent>
        </Card>
    );
}

export function RequestSummaryPage() {
    const [selectedPeriod, setSelectedPeriod] = useState<string>('today');
    const [dateRange, setDateRange] = useState<DateRange>(() => timePeriods[0].getDateRange());

    const startDateStr = `${dateRange.startDate}T00:00:00Z`;
    const endDateStr = `${dateRange.endDate}T23:59:59Z`;

    const { data: statistics, isLoading: loading, error, refetch } = useRequestStatistics(startDateStr, endDateStr);

    if (loading) {
        return (
            <div className="flex items-center justify-center py-20">
                <div className="text-center">
                    <div className="mx-auto mb-4 h-12 w-12 animate-spin rounded-full border-b-2 border-primary"></div>
                    <p className="text-muted-fg">Loading request summary...</p>
                </div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex items-center justify-center py-20">
                <div className="text-center" role="alert">
                    <div className="mb-4 text-6xl text-danger">⚠️</div>
                    <h1 className="mb-2 text-2xl font-semibold">Error Loading Request Summary</h1>
                    <p className="mb-4 text-muted-fg">{error instanceof Error ? error.message : 'Unknown error'}</p>
                    <Button onClick={() => refetch()}>Retry</Button>
                </div>
            </div>
        );
    }

    if (!statistics) {
        return (
            <div className="flex items-center justify-center py-20">
                <div className="text-center">
                    <p className="text-muted-fg">No request summary available</p>
                </div>
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-7xl space-y-6">
            <PageHeader
                title="Request Summary"
                description="Monitor gateway performance and traffic patterns"
                actions={
                    <>
                        <Button onClick={() => refetch()} disabled={loading}>
                            <span className={`${loading ? 'animate-spin' : ''}`}>{loading ? '⟳' : '🔄'}</span>
                            Refresh
                        </Button>
                        <StatisticsDateRange
                            dateRange={dateRange}
                            setDateRange={setDateRange}
                            selectedPeriod={selectedPeriod}
                            setSelectedPeriod={setSelectedPeriod}
                        />
                    </>
                }
            />
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-4">
                <StatisticCard
                    title="Total Requests"
                    value={statistics.totalRequests.toLocaleString()}
                    icon="📊"
                    accent="primary"
                />
                <StatisticCard
                    title="Avg Response Time"
                    value={`${statistics.averageResponseTime.toFixed(1)}ms`}
                    icon="⚡"
                    accent="success"
                />
                <StatisticCard
                    title="Avg Response Size"
                    value={`${(statistics.averageResponseSize / 1024).toFixed(1)}KB`}
                    icon="📦"
                    accent="warning"
                />
                <StatisticCard
                    title="Success Rate"
                    value={`${(
                        (Object.entries(statistics.requestsByStatus)
                            .filter(([status]) => status.startsWith('2') || status.startsWith('3'))
                            .reduce((sum, [, count]) => sum + count, 0) / statistics.totalRequests) *
                        100
                    ).toFixed(1)}%`}
                    icon="✅"
                    accent="success"
                />
            </div>
            <TrafficOverTimeSection start={new Date(startDateStr)} end={new Date(endDateStr)} />
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
                <DataTable title="Requests by Status Code" data={statistics.requestsByStatus} accent="primary" />
                <DataTable title="Requests by Country" data={statistics.requestsByCountry} accent="primary" />
                <DataTable title="Requests by Device Type" data={statistics.requestsByDeviceType} accent="primary" />
                <DataTable title="Requests by Platform" data={statistics.requestsByPlatform} accent="primary" />
                <DataTable title="Requests by Browser" data={statistics.requestsByBrowser} accent="primary" />
                {statistics.requestsByUser && (
                    <DataTable title="Requests by User" data={statistics.requestsByUser} accent="primary" />
                )}
                <DataTable
                    title="Requests by Fingerprint"
                    data={statistics.requestsByFingerprint}
                    accent="warning"
                />
                <DataTable
                    title="Requests by Fingerprint Type"
                    data={statistics.requestsByFingerprintType}
                    accent="warning"
                />
            </div>
        </div>
    );
}
