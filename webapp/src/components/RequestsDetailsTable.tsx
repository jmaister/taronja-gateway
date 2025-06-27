
export type RequestDetail = {
    id: string;
    timestamp: string;
    status_code: number;
    response_time: number;
    response_size: number;
    country: string;
    device_type: string;
    platform: string;
    browser: string;
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
    
    return (
        <div className="overflow-x-auto">
            <div className="mb-2 text-sm text-gray-600">
                Timezone: {Intl.DateTimeFormat().resolvedOptions().timeZone}
            </div>
            <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                    <tr>
                        <th className="px-4 py-2">ID</th>
                        <th className="px-4 py-2">Timestamp</th>
                        <th className="px-4 py-2">Status</th>
                        <th className="px-4 py-2 text-right">Resp. Time (ms)</th>
                        <th className="px-4 py-2 text-right">Resp. Size (B)</th>
                        <th className="px-4 py-2">Country</th>
                        <th className="px-4 py-2">Device</th>
                        <th className="px-4 py-2">Platform</th>
                        <th className="px-4 py-2">Browser</th>
                    </tr>
                </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                    {requests.map((req) => (
                        <tr key={req.id}>
                            <td className="px-4 py-2 whitespace-nowrap">{req.id}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{dateFormatter.format(new Date(req.timestamp))}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{req.status_code}</td>
                            <td className="px-4 py-2 whitespace-nowrap text-right">{decimalFormatter.format(req.response_time)}</td>
                            <td className="px-4 py-2 whitespace-nowrap text-right">{numberFormatter.format(req.response_size)}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{req.country}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{req.device_type}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{req.platform}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{req.browser}</td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}
