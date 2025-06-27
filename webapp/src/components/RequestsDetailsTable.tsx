
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
    return (
        <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                    <tr>
                        <th className="px-4 py-2">ID</th>
                        <th className="px-4 py-2">Timestamp</th>
                        <th className="px-4 py-2">Status</th>
                        <th className="px-4 py-2">Resp. Time (ms)</th>
                        <th className="px-4 py-2">Resp. Size (B)</th>
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
                            <td className="px-4 py-2 whitespace-nowrap">{req.timestamp}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{req.status_code}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{req.response_time}</td>
                            <td className="px-4 py-2 whitespace-nowrap">{req.response_size}</td>
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
