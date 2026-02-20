// Metrics page - Key performance metrics visualization
import { useEffect, useState, useRef } from 'react';
import { useStatusStore } from '../store';
import { BarChart3, Activity, Clock, Radio, Satellite, Shield } from 'lucide-react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Area,
  AreaChart,
} from 'recharts';

interface HistoryPoint {
  time: string;
  offset: number;
  jitter: number;
}

interface MetricCardProps {
  title: string;
  value: string | number;
  unit?: string;
  icon: React.ReactNode;
  trend?: 'up' | 'down' | 'stable';
  description?: string;
}

function MetricCard({ title, value, unit, icon, description }: MetricCardProps) {
  return (
    <div className="card">
      <div className="p-4">
        <div className="flex items-start justify-between">
          <div className="flex-1">
            <p className="text-sm text-gray-400">{title}</p>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-2xl font-bold text-white">{value}</span>
              {unit && <span className="text-sm text-gray-400">{unit}</span>}
            </div>
            {description && (
              <p className="mt-1 text-xs text-gray-500">{description}</p>
            )}
          </div>
          <div className="p-2 bg-gray-700 rounded-lg">{icon}</div>
        </div>
      </div>
    </div>
  );
}

const MAX_HISTORY_POINTS = 30;

export default function Metrics() {
  const { status } = useStatusStore();
  const [history, setHistory] = useState<HistoryPoint[]>([]);
  const lastUpdateRef = useRef<number>(0);

  // Track history from real-time status updates
  useEffect(() => {
    if (!status?.clock) return;

    const now = Date.now();
    // Only add a point every 60 seconds to match 30-minute window
    if (now - lastUpdateRef.current < 60000) return;
    lastUpdateRef.current = now;

    const newPoint: HistoryPoint = {
      time: new Date().toLocaleTimeString('en-US', { hour12: false }),
      offset: status.clock.offset_ns || 0,
      jitter: status.clock.offset_stddev_ns || 0,
    };

    setHistory(prev => {
      const updated = [...prev, newPoint];
      // Keep only last 30 points (30 minutes)
      if (updated.length > MAX_HISTORY_POINTS) {
        return updated.slice(-MAX_HISTORY_POINTS);
      }
      return updated;
    });
  }, [status?.clock?.offset_ns]);

  const formatDuration = (ns: number): string => {
    const abs = Math.abs(ns);
    if (abs < 1000) return `${ns.toFixed(0)}`;
    if (abs < 1000000) return `${(ns / 1000).toFixed(2)}`;
    return `${(ns / 1000000).toFixed(2)}`;
  };

  const formatDurationUnit = (ns: number): string => {
    const abs = Math.abs(ns);
    if (abs < 1000) return 'ns';
    if (abs < 1000000) return 'µs';
    return 'ms';
  };

  // Use real data or show placeholder message if no history yet
  const chartData = history.length > 0 ? history : [
    { time: new Date().toLocaleTimeString('en-US', { hour12: false }), offset: 0, jitter: 0 }
  ];

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold text-white">Metrics</h1>
        <p className="text-gray-400">Performance metrics and statistics</p>
      </div>

      {/* Key metrics grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricCard
          title="Current Offset"
          value={formatDuration(status?.clock?.offset_ns || 0)}
          unit={formatDurationUnit(status?.clock?.offset_ns || 0)}
          icon={<Clock className="w-5 h-5 text-primary-400" />}
          description="Offset from reference clock"
        />
        <MetricCard
          title="Clock Drift"
          value={(status?.clock?.drift_ppb || 0).toFixed(3)}
          unit="ppb"
          icon={<Activity className="w-5 h-5 text-yellow-400" />}
          description="Parts per billion"
        />
        <MetricCard
          title="NTP Requests"
          value={(status?.ntp?.request_count || 0).toLocaleString()}
          icon={<Radio className="w-5 h-5 text-green-400" />}
          description="Total requests handled"
        />
        <MetricCard
          title="GPS Satellites"
          value={status?.gpsdo?.satellites || 0}
          icon={<Satellite className="w-5 h-5 text-blue-400" />}
          description="Satellites in view"
        />
      </div>

      {/* Charts row */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Offset chart */}
        <div className="card">
          <div className="card-header flex items-center gap-2">
            <BarChart3 className="w-5 h-5" />
            Clock Offset (Last 30 minutes)
            {history.length === 0 && (
              <span className="text-xs text-gray-500 ml-2">(collecting data...)</span>
            )}
          </div>
          <div className="card-body">
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chartData}>
                  <defs>
                    <linearGradient id="offsetGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#0ea5e9" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#0ea5e9" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                  <XAxis
                    dataKey="time"
                    stroke="#9ca3af"
                    fontSize={12}
                    tickLine={false}
                  />
                  <YAxis
                    stroke="#9ca3af"
                    fontSize={12}
                    tickLine={false}
                    tickFormatter={(value) => `${value} ns`}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: '#1f2937',
                      border: '1px solid #374151',
                      borderRadius: '8px',
                    }}
                    labelStyle={{ color: '#9ca3af' }}
                  />
                  <Area
                    type="monotone"
                    dataKey="offset"
                    stroke="#0ea5e9"
                    fill="url(#offsetGradient)"
                    strokeWidth={2}
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>

        {/* Jitter chart */}
        <div className="card">
          <div className="card-header flex items-center gap-2">
            <Activity className="w-5 h-5" />
            Jitter (Last 30 minutes)
            {history.length === 0 && (
              <span className="text-xs text-gray-500 ml-2">(collecting data...)</span>
            )}
          </div>
          <div className="card-body">
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                  <XAxis
                    dataKey="time"
                    stroke="#9ca3af"
                    fontSize={12}
                    tickLine={false}
                  />
                  <YAxis
                    stroke="#9ca3af"
                    fontSize={12}
                    tickLine={false}
                    tickFormatter={(value) => `${value} ns`}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: '#1f2937',
                      border: '1px solid #374151',
                      borderRadius: '8px',
                    }}
                    labelStyle={{ color: '#9ca3af' }}
                  />
                  <Line
                    type="monotone"
                    dataKey="jitter"
                    stroke="#f59e0b"
                    strokeWidth={2}
                    dot={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>
      </div>

      {/* Detailed metrics */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* NTP Metrics */}
        <div className="card">
          <div className="card-header flex items-center gap-2">
            <Radio className="w-5 h-5" />
            NTP Metrics
          </div>
          <div className="card-body space-y-3">
            <div className="flex justify-between">
              <span className="text-gray-400">Stratum</span>
              <span className="text-white font-mono">{status?.ntp?.stratum || '-'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Root Delay</span>
              <span className="text-white font-mono">
                {status?.ntp?.root_delay_ms?.toFixed(3) || '-'} ms
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Root Dispersion</span>
              <span className="text-white font-mono">
                {status?.ntp?.root_dispersion_ms?.toFixed(3) || '-'} ms
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Peer Count</span>
              <span className="text-white font-mono">{status?.ntp?.peer_count || '-'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Total Requests</span>
              <span className="text-white font-mono">
                {status?.ntp?.request_count?.toLocaleString() || '-'}
              </span>
            </div>
          </div>
        </div>

        {/* PTP Metrics */}
        <div className="card">
          <div className="card-header flex items-center gap-2">
            <Activity className="w-5 h-5" />
            PTP Metrics
          </div>
          <div className="card-body space-y-3">
            <div className="flex justify-between">
              <span className="text-gray-400">Port State</span>
              <span className="text-white">{status?.ptp?.port_state || '-'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Clock Class</span>
              <span className="text-white font-mono">{status?.ptp?.clock_class || '-'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Sync Messages</span>
              <span className="text-white font-mono">
                {status?.ptp?.sync_count?.toLocaleString() || '-'}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Announce Messages</span>
              <span className="text-white font-mono">
                {status?.ptp?.announce_count?.toLocaleString() || '-'}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Delay Requests</span>
              <span className="text-white font-mono">
                {status?.ptp?.delay_req_count?.toLocaleString() || '-'}
              </span>
            </div>
          </div>
        </div>

        {/* NTS Metrics */}
        <div className="card">
          <div className="card-header flex items-center gap-2">
            <Shield className="w-5 h-5" />
            NTS Metrics
          </div>
          <div className="card-body space-y-3">
            <div className="flex justify-between">
              <span className="text-gray-400">Active Sessions</span>
              <span className="text-white font-mono">
                {status?.nts?.active_sessions || '-'}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">KE Requests</span>
              <span className="text-white font-mono">
                {status?.nts?.total_ke_requests?.toLocaleString() || '-'}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">KE Success</span>
              <span className="text-white font-mono">
                {status?.nts?.total_ke_success?.toLocaleString() || '-'}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">KE Failures</span>
              <span className="text-white font-mono">
                {status?.nts?.total_ke_failures?.toLocaleString() || '-'}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-400">Success Rate</span>
              <span className="text-white font-mono">
                {status?.nts && status.nts.total_ke_requests > 0
                  ? `${((status.nts.total_ke_success / status.nts.total_ke_requests) * 100).toFixed(1)}%`
                  : '-'}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Prometheus endpoint info */}
      <div className="card bg-gray-800/50">
        <div className="p-4">
          <div className="flex items-center gap-3">
            <BarChart3 className="w-6 h-6 text-primary-400" />
            <div>
              <p className="text-gray-200 font-medium">Prometheus Metrics</p>
              <p className="text-sm text-gray-400">
                Detailed metrics are available at <code className="px-1.5 py-0.5 bg-gray-700 rounded text-primary-300">/metrics</code> for Prometheus scraping
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
