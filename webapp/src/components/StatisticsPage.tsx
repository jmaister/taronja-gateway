import { useState, useEffect } from 'react';
import { RequestStatistics, fetchRequestStatistics } from '../services/api';
import { startOfWeek, endOfWeek, subWeeks, startOfMonth, endOfMonth, subMonths, startOfYear, endOfYear, format } from 'date-fns';

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

interface TimePeriod {
  label: string;
  value: string;
  getDateRange: () => { startDate: string; endDate: string };
}

// Use a fixed 'today' for all calculations in this render
const todayDate = new Date();
todayDate.setHours(0,0,0,0); // Remove time part for consistency

const timePeriods: TimePeriod[] = [
  {
    label: 'Today',
    value: 'today',
    getDateRange: () => {
      const dateStr = format(todayDate, 'yyyy-MM-dd');
      return { startDate: dateStr, endDate: dateStr };
    }
  },
  {
    label: 'Yesterday',
    value: 'yesterday',
    getDateRange: () => {
      const yesterday = new Date(todayDate);
      yesterday.setDate(todayDate.getDate() - 1);
      const dateStr = format(yesterday, 'yyyy-MM-dd');
      return { startDate: dateStr, endDate: dateStr };
    }
  },
  {
    label: 'This Week',
    value: 'thisWeek',
    getDateRange: () => {
      const start = startOfWeek(todayDate, { weekStartsOn: 1 });
      const end = endOfWeek(todayDate, { weekStartsOn: 1 });
      return {
        startDate: format(start, 'yyyy-MM-dd'),
        endDate: format(end, 'yyyy-MM-dd')
      };
    }
  },
  {
    label: 'Last Week',
    value: 'lastWeek',
    getDateRange: () => {
      const lastWeek = subWeeks(todayDate, 1);
      const start = startOfWeek(lastWeek, { weekStartsOn: 1 });
      const end = endOfWeek(lastWeek, { weekStartsOn: 1 });
      return {
        startDate: format(start, 'yyyy-MM-dd'),
        endDate: format(end, 'yyyy-MM-dd')
      };
    }
  },
  {
    label: 'This Month',
    value: 'thisMonth',
    getDateRange: () => {
      const start = startOfMonth(todayDate);
      const end = endOfMonth(todayDate);
      return {
        startDate: format(start, 'yyyy-MM-dd'),
        endDate: format(end, 'yyyy-MM-dd')
      };
    }
  },
  {
    label: 'Last Month',
    value: 'lastMonth',
    getDateRange: () => {
      const lastMonth = subMonths(todayDate, 1);
      const start = startOfMonth(lastMonth);
      const end = endOfMonth(lastMonth);
      return {
        startDate: format(start, 'yyyy-MM-dd'),
        endDate: format(end, 'yyyy-MM-dd')
      };
    }
  },
  {
    label: 'This Year',
    value: 'thisYear',
    getDateRange: () => {
      const start = startOfYear(todayDate);
      const end = endOfYear(todayDate);
      return {
        startDate: format(start, 'yyyy-MM-dd'),
        endDate: format(end, 'yyyy-MM-dd')
      };
    }
  },
  {
    label: 'Other...',
    value: 'other',
    getDateRange: () => {
      const dateStr = format(todayDate, 'yyyy-MM-dd');
      return { startDate: dateStr, endDate: dateStr };
    }
  }
];

export function StatisticsPage() {
  const [statistics, setStatistics] = useState<RequestStatistics | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedPeriod, setSelectedPeriod] = useState<string>('today');
  const [dateRange, setDateRange] = useState(() => {
    const today = timePeriods.find(p => p.value === 'today');
    return today ? today.getDateRange() : {
      startDate: new Date().toISOString().split('T')[0],
      endDate: new Date().toISOString().split('T')[0]
    };
  });

  const handlePeriodChange = (period: string) => {
    setSelectedPeriod(period);
    if (period !== 'other') {
      const timePeriod = timePeriods.find(p => p.value === period);
      if (timePeriod) {
        setDateRange(timePeriod.getDateRange());
      }
    }
    // For 'other', keep the current dateRange and let user modify it
  };

  const handleDateChange = (field: 'startDate' | 'endDate', value: string) => {
    setDateRange(prev => ({ ...prev, [field]: value }));
  };

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
  }, [dateRange]);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading statistics...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="text-red-500 text-6xl mb-4">⚠️</div>
          <h1 className="text-2xl font-bold text-gray-900 mb-2">Error Loading Statistics</h1>
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
          <p className="text-gray-600">No statistics available</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Request Statistics</h1>
          <p className="text-gray-600 mt-1">Monitor gateway performance and traffic patterns</p>
        </div>
        
        {/* Time Period Selector */}
        <div className="flex items-center space-x-4">
          <div className="flex items-center space-x-2">
            <label htmlFor="timePeriod" className="text-sm font-medium text-gray-700">Time Period:</label>
            <select
              id="timePeriod"
              value={selectedPeriod}
              onChange={(e) => handlePeriodChange(e.target.value)}
              className="px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 bg-white"
            >
              {timePeriods.map((period) => (
                <option key={period.value} value={period.value}>
                  {period.label}
                </option>
              ))}
            </select>
          </div>
          
          {selectedPeriod === 'other' ? (
            <>
              <div className="flex items-center space-x-2">
                <label htmlFor="startDate" className="text-sm font-medium text-gray-700">From:</label>
                <input
                  type="date"
                  id="startDate"
                  value={dateRange.startDate}
                  onChange={(e) => handleDateChange('startDate', e.target.value)}
                  className="px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div className="flex items-center space-x-2">
                <label htmlFor="endDate" className="text-sm font-medium text-gray-700">To:</label>
                <input
                  type="date"
                  id="endDate"
                  value={dateRange.endDate}
                  onChange={(e) => handleDateChange('endDate', e.target.value)}
                  className="px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
            </>
          ) : (
            <div className="text-sm text-gray-500">
              {new Date(dateRange.startDate).toLocaleDateString()} - {new Date(dateRange.endDate).toLocaleDateString()}
            </div>
          )}
        </div>
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
      </div>
    </div>
  );
}
