// Main App component for SPTime
import { useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuthStore, useStatusStore } from './store';
import { StatusWebSocket, api } from './api/client';
import Layout from './components/common/Layout';
import Dashboard from './pages/Dashboard';
import Sources from './pages/Sources';
import Configuration from './pages/Configuration';
import Logs from './pages/Logs';
import Metrics from './pages/Metrics';
import Login from './pages/Login';

const wsClient = new StatusWebSocket();

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuthStore();

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

export default function App() {
  const { isAuthenticated } = useAuthStore();
  const { setStatus, setPeers, setSatellites, setLogs, setConnected, setError } = useStatusStore();

  useEffect(() => {
    if (isAuthenticated) {
      // Connect WebSocket for real-time updates
      wsClient.connect(
        (status) => setStatus(status),
        (connected) => setConnected(connected)
      );

      // Fetch initial data
      const fetchData = async () => {
        try {
          const [peers, satellites, logs] = await Promise.all([
            api.getPeers(),
            api.getSatellites(),
            api.getLogs(),
          ]);
          setPeers(peers);
          setSatellites(satellites);
          setLogs(logs);
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Failed to fetch data');
        }
      };

      fetchData();

      // Refresh peers and logs periodically
      const interval = setInterval(async () => {
        try {
          const [peers, logs] = await Promise.all([
            api.getPeers(),
            api.getLogs(),
          ]);
          setPeers(peers);
          setLogs(logs);
        } catch (err) {
          console.error('Failed to refresh data:', err);
        }
      }, 10000);

      return () => {
        wsClient.disconnect();
        clearInterval(interval);
      };
    }
  }, [isAuthenticated, setStatus, setPeers, setSatellites, setLogs, setConnected, setError]);

  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<Dashboard />} />
                <Route path="/sources" element={<Sources />} />
                <Route path="/config" element={<Configuration />} />
                <Route path="/logs" element={<Logs />} />
                <Route path="/metrics" element={<Metrics />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}
