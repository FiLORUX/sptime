// Dashboard page - main overview of time synchronization status
import { useStatusStore } from '../store';
import { Clock, Radio, Shield, Satellite, TrendingUp, Activity } from 'lucide-react';
import clsx from 'clsx';

function formatDuration(ns: number): string {
  const abs = Math.abs(ns);
  if (abs < 1000) return `${ns.toFixed(0)} ns`;
  if (abs < 1000000) return `${(ns / 1000).toFixed(2)} µs`;
  if (abs < 1000000000) return `${(ns / 1000000).toFixed(2)} ms`;
  return `${(ns / 1000000000).toFixed(3)} s`;
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h ${mins}m`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

interface StatCardProps {
  title: string;
  value: string | number;
  subtitle?: string;
  icon: React.ReactNode;
  color?: 'blue' | 'green' | 'yellow' | 'red' | 'purple';
}

function StatCard({ title, value, subtitle, icon, color = 'blue' }: StatCardProps) {
  const colorClasses = {
    blue: 'from-blue-500 to-blue-600',
    green: 'from-green-500 to-green-600',
    yellow: 'from-yellow-500 to-yellow-600',
    red: 'from-red-500 to-red-600',
    purple: 'from-purple-500 to-purple-600',
  };

  return (
    <div className="card">
      <div className="p-4">
        <div className="flex items-start justify-between">
          <div>
            <p className="text-sm text-gray-400">{title}</p>
            <p className="mt-1 text-2xl font-bold text-white">{value}</p>
            {subtitle && <p className="mt-1 text-sm text-gray-400">{subtitle}</p>}
          </div>
          <div className={clsx('p-3 rounded-lg bg-gradient-to-br', colorClasses[color])}>
            {icon}
          </div>
        </div>
      </div>
    </div>
  );
}

interface StatusPanelProps {
  title: string;
  children: React.ReactNode;
  icon: React.ReactNode;
}

function StatusPanel({ title, children, icon }: StatusPanelProps) {
  return (
    <div className="card">
      <div className="card-header flex items-center gap-2">
        {icon}
        {title}
      </div>
      <div className="card-body">{children}</div>
    </div>
  );
}

export default function Dashboard() {
  const { status, loading, error } = useStatusStore();

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-500" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="card bg-red-900/20 border-red-700">
        <div className="p-4 text-red-400">
          <p className="font-medium">Error loading status</p>
          <p className="text-sm mt-1">{error}</p>
        </div>
      </div>
    );
  }

  if (!status) {
    return null;
  }

  const { clock, ntp, nts, ptp, gpsdo } = status;

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold text-white">Dashboard</h1>
        <p className="text-gray-400">Time synchronization overview</p>
      </div>

      {/* Main clock status */}
      <div className="card bg-gradient-to-br from-gray-800 to-gray-900">
        <div className="p-6">
          <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-6">
            {/* Current time display */}
            <div className="text-center lg:text-left">
              <p className="text-sm text-gray-400 uppercase tracking-wider">Server Time (UTC)</p>
              <p className="mt-2 text-4xl lg:text-5xl font-mono font-bold text-white">
                {new Date(status.time).toLocaleTimeString('en-US', { hour12: false })}
              </p>
              <p className="mt-1 text-lg text-gray-400 font-mono">
                {new Date(status.time).toLocaleDateString('en-US', {
                  weekday: 'long',
                  year: 'numeric',
                  month: 'long',
                  day: 'numeric',
                })}
              </p>
            </div>

            {/* Status and offset */}
            <div className="flex flex-wrap gap-6 justify-center lg:justify-end">
              <div className="text-center">
                <p className="text-sm text-gray-400">State</p>
                <div
                  className={clsx(
                    'mt-2 px-4 py-2 rounded-lg font-bold text-xl',
                    clock.state === 'LOCKED' && 'bg-green-900/50 text-green-400',
                    clock.state === 'SYNCING' && 'bg-yellow-900/50 text-yellow-400 animate-pulse-slow',
                    clock.state === 'HOLDOVER' && 'bg-orange-900/50 text-orange-400',
                    clock.state === 'FAULT' && 'bg-red-900/50 text-red-400',
                    clock.state === 'INIT' && 'bg-blue-900/50 text-blue-400'
                  )}
                >
                  {clock.state}
                </div>
              </div>

              <div className="text-center">
                <p className="text-sm text-gray-400">Offset</p>
                <p className="mt-2 text-2xl font-mono font-bold text-white">
                  {formatDuration(clock.offset_ns)}
                </p>
              </div>

              <div className="text-center">
                <p className="text-sm text-gray-400">Stratum</p>
                <p className="mt-2 text-2xl font-bold text-white">{clock.stratum}</p>
              </div>

              <div className="text-center">
                <p className="text-sm text-gray-400">Reference</p>
                <p className="mt-2 text-lg font-medium text-primary-400">
                  {clock.active_reference || 'None'}
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Stats grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="Uptime"
          value={formatUptime(status.uptime_seconds)}
          icon={<Clock className="w-6 h-6 text-white" />}
          color="blue"
        />
        <StatCard
          title="NTP Requests"
          value={ntp?.request_count?.toLocaleString() ?? 'N/A'}
          subtitle={ntp?.running ? 'Server running' : 'Server stopped'}
          icon={<Activity className="w-6 h-6 text-white" />}
          color="green"
        />
        <StatCard
          title="Active NTS Sessions"
          value={nts?.active_sessions ?? 'N/A'}
          subtitle={nts?.running ? 'NTS enabled' : 'NTS disabled'}
          icon={<Shield className="w-6 h-6 text-white" />}
          color="purple"
        />
        <StatCard
          title="Drift"
          value={`${clock.drift_ppb.toFixed(3)} ppb`}
          icon={<TrendingUp className="w-6 h-6 text-white" />}
          color="yellow"
        />
      </div>

      {/* Status panels grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* NTP Status */}
        <StatusPanel title="NTP Server" icon={<Radio className="w-5 h-5" />}>
          {ntp ? (
            <div className="space-y-3">
              <div className="flex justify-between">
                <span className="text-gray-400">Status</span>
                <span className={ntp.running ? 'text-green-400' : 'text-red-400'}>
                  {ntp.running ? 'Running' : 'Stopped'}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Stratum</span>
                <span className="text-white">{ntp.stratum}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Reference ID</span>
                <span className="text-white font-mono">{ntp.ref_id}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Peer Count</span>
                <span className="text-white">{ntp.peer_count}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Root Delay</span>
                <span className="text-white font-mono">{ntp.root_delay_ms.toFixed(3)} ms</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Root Dispersion</span>
                <span className="text-white font-mono">{ntp.root_dispersion_ms.toFixed(3)} ms</span>
              </div>
            </div>
          ) : (
            <p className="text-gray-400">NTP server not configured</p>
          )}
        </StatusPanel>

        {/* PTP Status */}
        <StatusPanel title="PTP Grandmaster" icon={<Activity className="w-5 h-5" />}>
          {ptp ? (
            <div className="space-y-3">
              <div className="flex justify-between">
                <span className="text-gray-400">Status</span>
                <span className={ptp.running ? 'text-green-400' : 'text-red-400'}>
                  {ptp.running ? 'Running' : 'Stopped'}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Port State</span>
                <span className="text-white">{ptp.port_state}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Domain</span>
                <span className="text-white">{ptp.domain}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Clock Class</span>
                <span className="text-white">{ptp.clock_class}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Priority</span>
                <span className="text-white">{ptp.priority1} / {ptp.priority2}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Sync Messages</span>
                <span className="text-white font-mono">{ptp.sync_count.toLocaleString()}</span>
              </div>
            </div>
          ) : (
            <p className="text-gray-400">PTP not configured</p>
          )}
        </StatusPanel>

        {/* NTS Status */}
        <StatusPanel title="NTS (Network Time Security)" icon={<Shield className="w-5 h-5" />}>
          {nts ? (
            <div className="space-y-3">
              <div className="flex justify-between">
                <span className="text-gray-400">Status</span>
                <span className={nts.running ? 'text-green-400' : 'text-red-400'}>
                  {nts.running ? 'Running' : 'Stopped'}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">KE Port</span>
                <span className="text-white">{nts.ke_port}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Active Sessions</span>
                <span className="text-white">{nts.active_sessions}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Total KE Requests</span>
                <span className="text-white font-mono">{nts.total_ke_requests.toLocaleString()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Success Rate</span>
                <span className="text-white">
                  {nts.total_ke_requests > 0
                    ? `${((nts.total_ke_success / nts.total_ke_requests) * 100).toFixed(1)}%`
                    : 'N/A'}
                </span>
              </div>
            </div>
          ) : (
            <p className="text-gray-400">NTS not configured</p>
          )}
        </StatusPanel>

        {/* GPSDO Status */}
        <StatusPanel title="GPSDO" icon={<Satellite className="w-5 h-5" />}>
          {gpsdo ? (
            <div className="space-y-3">
              <div className="flex justify-between">
                <span className="text-gray-400">Type</span>
                <span className="text-white capitalize">{gpsdo.type}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Status</span>
                <span className={gpsdo.locked ? 'text-green-400' : 'text-yellow-400'}>
                  {gpsdo.locked ? 'Locked' : 'Acquiring'}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Satellites</span>
                <span className="text-white">{gpsdo.satellites}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Signal Strength</span>
                <span className="text-white font-mono">{gpsdo.signal_strength_db.toFixed(1)} dB</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">PPS Count</span>
                <span className="text-white font-mono">{gpsdo.pps_count.toLocaleString()}</span>
              </div>
              {gpsdo.latitude !== undefined && gpsdo.longitude !== undefined && (
                <div className="flex justify-between">
                  <span className="text-gray-400">Position</span>
                  <span className="text-white font-mono text-sm">
                    {gpsdo.latitude.toFixed(4)}, {gpsdo.longitude.toFixed(4)}
                  </span>
                </div>
              )}
            </div>
          ) : (
            <p className="text-gray-400">GPSDO not configured</p>
          )}
        </StatusPanel>
      </div>
    </div>
  );
}
