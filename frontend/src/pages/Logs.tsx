// Logs page - System event logs
import { useState, useMemo } from 'react';
import { useStatusStore } from '../store';
import { FileText, Filter, Search, AlertTriangle, Info, AlertCircle, Bug } from 'lucide-react';
import clsx from 'clsx';
import type { LogEntry } from '../types';

const levelIcons: Record<string, React.ReactNode> = {
  debug: <Bug className="w-4 h-4 text-gray-400" />,
  info: <Info className="w-4 h-4 text-blue-400" />,
  warning: <AlertTriangle className="w-4 h-4 text-yellow-400" />,
  warn: <AlertTriangle className="w-4 h-4 text-yellow-400" />,
  error: <AlertCircle className="w-4 h-4 text-red-400" />,
};

const levelColors: Record<string, string> = {
  debug: 'text-gray-400',
  info: 'text-blue-400',
  warning: 'text-yellow-400',
  warn: 'text-yellow-400',
  error: 'text-red-400',
};

function formatTimestamp(timestamp: string): string {
  return new Date(timestamp).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

export default function Logs() {
  const { logs } = useStatusStore();
  const [filter, setFilter] = useState<string>('all');
  const [search, setSearch] = useState('');

  const filteredLogs = useMemo(() => {
    return logs.filter((log) => {
      // Level filter
      if (filter !== 'all' && log.level !== filter) {
        return false;
      }

      // Search filter
      if (search) {
        const searchLower = search.toLowerCase();
        return (
          log.message.toLowerCase().includes(searchLower) ||
          log.component.toLowerCase().includes(searchLower) ||
          (log.fields && JSON.stringify(log.fields).toLowerCase().includes(searchLower))
        );
      }

      return true;
    });
  }, [logs, filter, search]);

  const logCounts = useMemo(() => {
    const counts: Record<string, number> = { all: logs.length };
    logs.forEach((log) => {
      counts[log.level] = (counts[log.level] || 0) + 1;
    });
    return counts;
  }, [logs]);

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold text-white">System Logs</h1>
        <p className="text-gray-400">View system events and errors</p>
      </div>

      {/* Filters */}
      <div className="flex flex-col md:flex-row gap-4">
        {/* Level filter */}
        <div className="flex items-center gap-2">
          <Filter className="w-5 h-5 text-gray-400" />
          <div className="flex gap-1">
            {['all', 'error', 'warning', 'info', 'debug'].map((level) => (
              <button
                key={level}
                onClick={() => setFilter(level)}
                className={clsx(
                  'px-3 py-1.5 rounded-md text-sm font-medium transition-colors',
                  filter === level
                    ? 'bg-primary-600 text-white'
                    : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
                )}
              >
                {level.charAt(0).toUpperCase() + level.slice(1)}
                {logCounts[level] !== undefined && (
                  <span className="ml-1.5 text-xs opacity-70">
                    ({logCounts[level] || 0})
                  </span>
                )}
              </button>
            ))}
          </div>
        </div>

        {/* Search */}
        <div className="flex-1 md:max-w-md">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search logs..."
              className="input pl-10"
            />
          </div>
        </div>
      </div>

      {/* Logs table */}
      <div className="card overflow-hidden">
        <div className="card-header flex items-center gap-2">
          <FileText className="w-5 h-5" />
          Log Entries ({filteredLogs.length})
        </div>
        <div className="overflow-x-auto max-h-[600px] overflow-y-auto">
          <table className="w-full">
            <thead className="sticky top-0 bg-gray-800 z-10">
              <tr className="border-b border-gray-700">
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400 w-44">
                  Timestamp
                </th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400 w-24">
                  Level
                </th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400 w-32">
                  Component
                </th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">
                  Message
                </th>
              </tr>
            </thead>
            <tbody>
              {filteredLogs.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-gray-400">
                    No log entries match your filters
                  </td>
                </tr>
              ) : (
                filteredLogs.map((log, idx) => (
                  <LogRow key={`${log.timestamp}-${idx}`} log={log} />
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

function LogRow({ log }: { log: LogEntry }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <>
      <tr
        className={clsx(
          'border-b border-gray-700/50 hover:bg-gray-700/30 cursor-pointer',
          log.level === 'error' && 'bg-red-900/10',
          log.level === 'warning' && 'bg-yellow-900/10'
        )}
        onClick={() => log.fields && setExpanded(!expanded)}
      >
        <td className="px-4 py-3 font-mono text-sm text-gray-300">
          {formatTimestamp(log.timestamp)}
        </td>
        <td className="px-4 py-3">
          <div className="flex items-center gap-2">
            {levelIcons[log.level]}
            <span className={clsx('text-sm font-medium uppercase', levelColors[log.level])}>
              {log.level}
            </span>
          </div>
        </td>
        <td className="px-4 py-3">
          <span className="px-2 py-1 text-xs font-medium bg-gray-700 rounded">
            {log.component}
          </span>
        </td>
        <td className="px-4 py-3 text-gray-200">
          <div className="flex items-center gap-2">
            {log.message}
            {log.fields && Object.keys(log.fields).length > 0 && (
              <span className="text-xs text-gray-400">
                ({Object.keys(log.fields).length} fields)
              </span>
            )}
          </div>
        </td>
      </tr>
      {expanded && log.fields && (
        <tr className="bg-gray-800/50">
          <td colSpan={4} className="px-4 py-3">
            <pre className="text-sm text-gray-300 overflow-x-auto">
              {JSON.stringify(log.fields, null, 2)}
            </pre>
          </td>
        </tr>
      )}
    </>
  );
}
