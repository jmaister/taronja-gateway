
export type RequestDetail = {
    id: string;
    timestamp: string;
    path: string;
    user_id?: string | null;
    username?: string | null;
    status_code: number;
    response_time: number;
    response_size: number;
    country: string;
    city: string;
    latitude?: number | null;
    longitude?: number | null;
    device_type: string;
    platform: string;
    platform_version: string;
    browser: string;
    browser_version: string;
};

export function RequestsDetailsTable({ requests }: { requests: RequestDetail[] }) {
    const numberFormatter = new Intl.NumberFormat(navigator.language);
    const decimalFormatter = new Intl.NumberFormat(navigator.language, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    const dateFormatter = new Intl.DateTimeFormat(navigator.language, { 
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
            <div className="mb-2 text-sm text-gray-600 flex flex-wrap gap-4">
                <span>Timezone: {Intl.DateTimeFormat().resolvedOptions().timeZone}</span>
                <span>Requests: {numberFormatter.format(totalRequests)}</span>
                <span>Total Time: {formatTime(totalTime)}</span>
                <span>Total Size: {formatBytes(totalBytes)}</span>
            </div>
            <div className="overflow-x-auto border border-gray-200 rounded-lg shadow-sm">
                <table className="min-w-full divide-y divide-gray-200">
                    <thead className="bg-gray-50">
                        <tr>
                            <th className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider min-w-36">Timestamp</th>
                            <th className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider min-w-48">Path</th>
                            <th className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider min-w-24">User</th>
                            <th className="px-2 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-16">Status</th>
                            <th className="px-2 py-2 text-right text-xs font-medium text-gray-500 uppercase tracking-wider w-20">Time (ms)</th>
                            <th className="px-2 py-2 text-right text-xs font-medium text-gray-500 uppercase tracking-wider w-20">Size (B)</th>
                            <th className="px-2 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-16">Country</th>
                            <th className="px-2 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider w-16">City</th>
                            <th className="px-2 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider min-w-20">Device</th>
                            <th className="px-2 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider min-w-24">Platform</th>
                            <th className="px-2 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider min-w-24">Browser</th>
                        </tr>
                    </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                    {requests.map((req) => (
                        <tr key={req.id}>
                            <td className="px-3 py-2 whitespace-nowrap text-sm">{dateFormatter.format(new Date(req.timestamp))}</td>
                            <td className="px-3 py-2 text-sm">
                                <div className="max-w-48 truncate">
                                    <a href={req.path} target="_blank" rel="noopener noreferrer" 
                                       className="text-blue-600 hover:underline" title={req.path}>
                                        {req.path}
                                    </a>
                                </div>
                            </td>
                            <td className="px-3 py-2 whitespace-nowrap text-sm">
                                {req.username ? (
                                    <a 
                                        href={`/_admin/users/${req.user_id}`} 
                                        className="text-blue-600 hover:underline"
                                        title={`User ID: ${req.user_id}`}
                                    >
                                        {req.username}
                                    </a>
                                ) : (
                                    <span className="text-gray-400">-</span>
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
