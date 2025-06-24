import { Bar } from 'react-chartjs-2';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Title,
  Tooltip,
  Legend,
  ChartData,
  ChartOptions,
} from 'chart.js';

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  BarElement,
  Title,
  Tooltip,
  Legend
);

const SampleBarChart = () => {
  const data: ChartData<'bar'> = {
    labels: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'],
    datasets: [
      {
        label: 'Page Views',
        data: [1200, 1900, 1500, 2100, 1800, 2200, 1700],
        backgroundColor: 'rgba(54, 162, 235, 0.7)', // Blue
        borderColor: 'rgba(54, 162, 235, 1)',
        borderWidth: 1,
        borderRadius: 4,
        barThickness: 20, // Example: Set bar thickness
      },
      {
        label: 'Unique Visitors',
        data: [800, 1200, 1000, 1500, 1100, 1400, 1050],
        backgroundColor: 'rgba(75, 192, 192, 0.7)', // Teal/Green
        borderColor: 'rgba(75, 192, 192, 1)',
        borderWidth: 1,
        borderRadius: 4,
        barThickness: 20, // Example: Set bar thickness
      },
    ],
  };

  const options: ChartOptions<'bar'> = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'bottom' as const, // Position legend at the bottom
        labels: {
          color: '#4A5568', // Tailwind gray-700
          font: {
            size: 12,
            family: 'Inter, sans-serif', // Consistent font
          },
          padding: 20, // Add padding to legend
        }
      },
      title: {
        display: true,
        text: 'Website Traffic Overview (Last 7 Days)',
        color: '#2D3748', // Tailwind gray-800
        font: {
          size: 16,
          family: 'Inter, sans-serif',
          weight: 600,
        },
        padding: {
          top: 10,
          bottom: 25, // Increased bottom padding
        }
      },
      tooltip: {
        backgroundColor: 'rgba(0,0,0,0.8)',
        titleFont: { family: 'Inter, sans-serif', size: 14 },
        bodyFont: { family: 'Inter, sans-serif', size: 12 },
        padding: 10,
        callbacks: {
          label: function(context) {
            let label = context.dataset.label || '';
            if (label) {
              label += ': ';
            }
            if (context.parsed.y !== null) {
              label += new Intl.NumberFormat('en-US').format(context.parsed.y);
            }
            return label;
          }
        }
      }
    },
    scales: {
      y: {
        beginAtZero: true,
        grid: {
          color: '#E2E8F0', // Tailwind gray-300 (stroke)
        },
        ticks: {
          color: '#718096', // Tailwind gray-500 (text)
          font: { family: 'Inter, sans-serif', size: 11 },
          callback: function(value) {
            if (Number(value) >= 1000) {
              return (Number(value) / 1000) + 'K';
            }
            return Number(value);
          }
        }
      },
      x: {
        grid: {
          display: false, // Cleaner look without vertical grid lines
        },
        ticks: {
          color: '#718096', // Tailwind gray-500
          font: { family: 'Inter, sans-serif', size: 11 },
        }
      }
    },
    interaction: {
      mode: 'index' as const,
      intersect: false,
    },
    // elements: { // Potential further global styling for bars
    //   bar: {
    //     borderSkipped: 'bottom',
    //   }
    // }
  };

  return (
    // Parent container needs to have a defined height for chart to be visible
    <div style={{ height: '100%', width: '100%' }}>
      <Bar data={data} options={options} />
    </div>
  );
};

export default SampleBarChart;
