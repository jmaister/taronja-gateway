import { RequestDetail } from "@/apiclient";
import { getSafeLocale } from "../lib/intl";


export function RequestsDetailsTable({ requests }: { requests: RequestDetail[] }) {
    const locale = getSafeLocale();

    const numberFormatter = new Intl.NumberFormat(locale);
    const decimalFormatter = new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    const dateFormatter = new Intl.DateTimeFormat(locale, {
        year: 'numeric', 
        month: '2-digit', 
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
    });

    // Calculate summary statistics
    const totalRequests = requests.length;
    const totalTime = requests.reduce((sum, req) => sum + req.response_time, 0);
    const totalBytes = requests.reduce((sum, req) => sum + req.response_size, 0);

    // Format bytes with appropriate units
    const formatBytes = (bytes: number): string => {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };

    // Format time with appropriate units
    const formatTime = (ms: number): string => {
        if (ms < 1000) return `${decimalFormatter.format(ms)} ms`;
        if (ms < 60000) return `${decimalFormatter.format(ms / 1000)} s`;
        if (ms < 3600000) return `${decimalFormatter.format(ms / 60000)} min`;
        return `${decimalFormatter.format(ms / 3600000)} h`;
    };
    
    return (
        <div className="w-full">
            <div className="mb-2 flex flex-wrap gap-4 text-sm text-muted-fg">
                <span>Timezone: {Intl.DateTimeFormat().resolvedOptions().timeZone}</span>
                <span>Requests: {numberFormatter.format(totalRequests)}</span>
                <span>Total Time: {formatTime(totalTime)}</span>
                <span>Total Size: {formatBytes(totalBytes)}</span>
            </div>
            <div className="overflow-x-auto rounded-lg border border-border bg-surface shadow-soft">
                <table className="min-w-full border-collapse">
                    <thead className="bg-surface-2">
                        <tr>
                            <th className="min-w-36 border-b border-border px-3 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">Timestamp</th>
                            <th className="min-w-48 border-b border-border px-3 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">Path</th>
                            <th className="min-w-24 border-b border-border px-3 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">User</th>
                            <th className="w-16 border-b border-border px-2 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">Status</th>
                            <th className="w-20 border-b border-border px-2 py-2 text-right text-xs font-semibold uppercase tracking-wider text-muted-fg">Time (ms)</th>
                            <th className="w-20 border-b border-border px-2 py-2 text-right text-xs font-semibold uppercase tracking-wider text-muted-fg">Size (B)</th>
                            <th className="w-16 border-b border-border px-2 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">Country</th>
                            <th className="w-16 border-b border-border px-2 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">City</th>
                            <th className="min-w-20 border-b border-border px-2 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">Device</th>
                            <th className="min-w-24 border-b border-border px-2 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">Platform</th>
                            <th className="min-w-24 border-b border-border px-2 py-2 text-left text-xs font-semibold uppercase tracking-wider text-muted-fg">Browser</th>
                        </tr>
                    </thead>
                <tbody className="divide-y divide-border">
                    {requests.map((req) => (
                        <tr key={req.id}>
                            <td className="whitespace-nowrap px-3 py-2 text-sm">{dateFormatter.format(new Date(req.timestamp))}</td>
                            <td className="px-3 py-2 text-sm">
                                <div className="max-w-48 truncate">
                                    <a href={req.path} target="_blank" rel="noopener noreferrer" 
                                       className="tg-link" title={req.path}>
                                        {req.path}
                                    </a>
                                </div>
                            </td>
                            <td className="px-3 py-2 whitespace-nowrap text-sm">
                                {req.username ? (
                                    <a 
                                        href={`/_/admin/users/${req.user_id}`} 
                                        className="tg-link"
                                        title={`User ID: ${req.user_id}`}
                                    >
                                        {req.username}
                                    </a>
                                ) : (
                                    <span className="text-muted-fg">-</span>
                                )}
                            </td>
                            <td className="px-2 py-2 whitespace-nowrap text-sm text-center">{req.status_code}</td>
                            <td className="px-2 py-2 whitespace-nowrap text-sm text-right">{decimalFormatter.format(req.response_time)}</td>
                            <td className="px-2 py-2 whitespace-nowrap text-sm text-right">{numberFormatter.format(req.response_size)}</td>
                            <td className="px-2 py-2 whitespace-nowrap text-sm text-center">{req.country}</td>
                            <td className="px-2 py-2 whitespace-nowrap text-sm text-center">{req.city}</td>
                            <td className="px-2 py-2 text-sm" title={req.device_type}>
                                <div className="max-w-20 truncate">{req.device_type}</div>
                            </td>
                            <td className="px-2 py-2 text-sm" title={`${req.platform} ${req.platform_version}`}>
                                <div className="max-w-24 truncate">
                                    {req.platform} {req.platform_version && `v${req.platform_version}`}
                                </div>
                            </td>
                            <td className="px-2 py-2 text-sm" title={`${req.browser} ${req.browser_version}`}>
                                <div className="max-w-24 truncate">
                                    {req.browser} {req.browser_version && `v${req.browser_version}`}
                                </div>
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
        </div>
    );
}
