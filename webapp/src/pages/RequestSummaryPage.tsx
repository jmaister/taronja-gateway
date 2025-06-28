import { useState, useEffect } from 'react';
import { RequestStatistics, fetchRequestStatistics } from '../services/api';
import { StatisticsDateRange, timePeriods, DateRange } from '../components/StatisticsDateRange';

interface StatCard {
  title: string;
  value: string | number;
  icon: string;
  color: string;
}

function StatisticCard({ title, value, icon, color }: StatCard) {
  return (
    <div className="bg-white rounded-lg shadow-md p-6 border-l-4" style={{ borderLeftColor: color }}>
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium text-gray-600">{title}</p>
          <p className="text-2xl font-bold text-gray-900">{value}</p>
        </div>
        <div className="text-3xl" style={{ color }}>
          {icon}
        </div>
      </div>
    </div>
  );
}

interface DataTableProps {
  title: string;
  data: Record<string, number>;
  color: string;
}

function DataTable({ title, data, color }: DataTableProps) {
  const sortedData = Object.entries(data)
    .filter(([key]) => key && key.trim() !== '')
    .sort(([, a], [, b]) => b - a)
    .slice(0, 10); // Show top 10

  if (sortedData.length === 0) {
    return (
      <div className="bg-white rounded-lg shadow-md p-6">
        <h3 className="text-lg font-semibold text-gray-900 mb-4" style={{ color }}>
          {title}
        </h3>
        <p className="text-gray-500 text-center py-8">No data available</p>
      </div>
    );
  }

  return (
    <div className="bg-white rounded-lg shadow-md p-6">
      <h3 className="text-lg font-semibold text-gray-900 mb-4" style={{ color }}>
        {title}
      </h3>
      <div className="space-y-2">
        {sortedData.map(([key, value], index) => (
          <div key={key} className="flex justify-between items-center py-2 border-b border-gray-100 last:border-b-0">
            <span className="text-sm font-medium text-gray-700">{key}</span>
            <div className="flex items-center space-x-2">
              <span className="text-sm font-bold text-gray-900">{value.toLocaleString()}</span>
              <div 
                className="w-2 h-2 rounded-full" 
                style={{ backgroundColor: color, opacity: 1 - (index * 0.1) }}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export function RequestSummaryPage() {
  const [statistics, setStatistics] = useState<RequestStatistics | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedPeriod, setSelectedPeriod] = useState<string>('today');
  const [dateRange, setDateRange] = useState<DateRange>(() => timePeriods[0].getDateRange());

  const loadStatistics = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await fetchRequestStatistics(
        `${dateRange.startDate}T00:00:00Z`,
        `${dateRange.endDate}T23:59:59Z`
      );
      setStatistics(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load statistics');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadStatistics();
    // eslint-disable-next-line
  }, [dateRange]);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading request summary...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="text-red-500 text-6xl mb-4">⚠️</div>
          <h1 className="text-2xl font-bold text-gray-900 mb-2">Error Loading Request Summary</h1>
          <p className="text-gray-600 mb-4">{error}</p>
          <button
            onClick={loadStatistics}
            className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 transition-colors"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (!statistics) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <p className="text-gray-600">No request summary available</p>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Request Summary</h1>
          <p className="text-gray-600 mt-1">Monitor gateway performance and traffic patterns</p>
        </div>
        <StatisticsDateRange
          dateRange={dateRange}
          setDateRange={setDateRange}
          selectedPeriod={selectedPeriod}
          setSelectedPeriod={setSelectedPeriod}
        />
      </div>
      {/* Key Metrics Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <StatisticCard
          title="Total Requests"
          value={statistics.totalRequests.toLocaleString()}
          icon="📊"
          color="#3B82F6"
        />
        <StatisticCard
          title="Avg Response Time"
          value={`${statistics.averageResponseTime.toFixed(1)}ms`}
          icon="⚡"
          color="#10B981"
        />
        <StatisticCard
          title="Avg Response Size"
          value={`${(statistics.averageResponseSize / 1024).toFixed(1)}KB`}
          icon="📦"
          color="#F59E0B"
        />
        <StatisticCard
          title="Success Rate"
          value={`${(
            (Object.entries(statistics.requestsByStatus)
              .filter(([status]) => status.startsWith('2') || status.startsWith('3'))
              .reduce((sum, [, count]) => sum + count, 0) / statistics.totalRequests) * 100
          ).toFixed(1)}%`}
          icon="✅"
          color="#EF4444"
        />
      </div>
      {/* Data Tables Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <DataTable
          title="Requests by Status Code"
          data={statistics.requestsByStatus}
          color="#8B5CF6"
        />
        <DataTable
          title="Requests by Country"
          data={statistics.requestsByCountry}
          color="#06B6D4"
        />
        <DataTable
          title="Requests by Device Type"
          data={statistics.requestsByDeviceType}
          color="#84CC16"
        />
        <DataTable
          title="Requests by Platform"
          data={statistics.requestsByPlatform}
          color="#F97316"
        />
        <DataTable
          title="Requests by Browser"
          data={statistics.requestsByBrowser}
          color="#EC4899"
        />
        {statistics.requestsByUser && (
          <DataTable
            title="Requests by User"
            data={statistics.requestsByUser}
            color="#0EA5E9"
          />
        )}
      </div>
    </div>
  );
}
