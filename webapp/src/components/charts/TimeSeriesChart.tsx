import { Line } from 'react-chartjs-2';
import {
    Chart as ChartJS,
    CategoryScale,
    LinearScale,
    LineElement,
    PointElement,
    Title,
    Tooltip,
    Legend,
    Filler,
    ChartData,
    ChartOptions,
} from 'chart.js';
import { format, parseISO } from 'date-fns';
import type { TimeSeriesPoint, TimeSeriesGranularity } from '@/apiclient';

ChartJS.register(CategoryScale, LinearScale, LineElement, PointElement, Title, Tooltip, Legend, Filler);

// No chartjs-adapter-date-fns dependency in this project (see webapp/package.json),
// so the X axis uses a plain CategoryScale with pre-formatted string labels
// instead of chart.js's built-in TimeScale — one label per bucket, formatted
// to match how coarse the bucket already is (no point showing a full date
// on an hourly chart, or a time-of-day on a monthly one).
function formatBucketLabel(timestamp: string, granularity: TimeSeriesGranularity): string {
    const date = parseISO(timestamp);
    switch (granularity) {
        case 'minute':
            return format(date, 'HH:mm');
        case 'hour':
            return format(date, 'MMM d, HH:mm');
        case 'day':
            return format(date, 'MMM d');
        case 'week':
            return format(date, 'MMM d');
        case 'month':
            return format(date, 'MMM yyyy');
    }
}

export interface TimeSeriesLine {
    /** Which TimeSeriesPoint numeric field this line plots. */
    key: 'requestCount' | 'uniqueFingerprints' | 'uniqueUsers' | 'errorCount' | 'averageResponseTime';
    label: string;
    color: string;
}

interface TimeSeriesChartProps {
    title: string;
    points: TimeSeriesPoint[];
    granularity: TimeSeriesGranularity;
    lines: TimeSeriesLine[];
    /** e.g. "ms" — appended to tooltip values for this chart's unit. */
    unit?: string;
}

export function TimeSeriesChart({ title, points, granularity, lines, unit }: TimeSeriesChartProps) {
    const labels = points.map((p) => formatBucketLabel(p.timestamp, granularity));

    const data: ChartData<'line'> = {
        labels,
        datasets: lines.map((line) => ({
            label: line.label,
            data: points.map((p) => p[line.key]),
            borderColor: line.color,
            backgroundColor: line.color,
            tension: 0.3,
            pointRadius: points.length > 60 ? 0 : 2,
            borderWidth: 2,
            fill: false,
        })),
    };

    const options: ChartOptions<'line'> = {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
            legend: {
                position: 'bottom',
                labels: {
                    color: '#4A5568',
                    font: { size: 12, family: 'Inter, sans-serif' },
                    padding: 16,
                },
            },
            title: {
                display: true,
                text: title,
                color: '#2D3748',
                font: { size: 16, family: 'Inter, sans-serif', weight: 600 },
                padding: { top: 10, bottom: 20 },
            },
            tooltip: {
                backgroundColor: 'rgba(0,0,0,0.8)',
                titleFont: { family: 'Inter, sans-serif', size: 13 },
                bodyFont: { family: 'Inter, sans-serif', size: 12 },
                padding: 10,
                callbacks: {
                    label: (context) => {
                        let label = context.dataset.label || '';
                        if (label) label += ': ';
                        if (context.parsed.y !== null) {
                            label += new Intl.NumberFormat('en-US').format(context.parsed.y);
                            if (unit) label += unit;
                        }
                        return label;
                    },
                },
            },
        },
        scales: {
            y: {
                beginAtZero: true,
                grid: { color: '#E2E8F0' },
                ticks: {
                    color: '#718096',
                    font: { family: 'Inter, sans-serif', size: 11 },
                },
            },
            x: {
                grid: { display: false },
                ticks: {
                    color: '#718096',
                    font: { family: 'Inter, sans-serif', size: 11 },
                    maxRotation: 0,
                    autoSkip: true,
                    maxTicksLimit: 12,
                },
            },
        },
        interaction: { mode: 'index', intersect: false },
    };

    return (
        <div style={{ height: '280px', width: '100%' }}>
            <Line data={data} options={options} />
        </div>
    );
}
