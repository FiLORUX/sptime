// Zustand store for SPTime application state
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { SystemStatus, User, PeerStatus, Satellite, LogEntry, Config } from '../types';

interface AuthState {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  login: (token: string, user: User) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      token: null,
      isAuthenticated: false,
      login: (token, user) => {
        localStorage.setItem('token', token);
        set({ token, user, isAuthenticated: true });
      },
      logout: () => {
        localStorage.removeItem('token');
        set({ token: null, user: null, isAuthenticated: false });
      },
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({ token: state.token, user: state.user }),
    }
  )
);

interface StatusState {
  status: SystemStatus | null;
  peers: PeerStatus[];
  satellites: Satellite[];
  logs: LogEntry[];
  config: Config | null;
  connected: boolean;
  loading: boolean;
  error: string | null;
  setStatus: (status: SystemStatus) => void;
  setPeers: (peers: PeerStatus[]) => void;
  setSatellites: (satellites: Satellite[]) => void;
  setLogs: (logs: LogEntry[]) => void;
  setConfig: (config: Config) => void;
  setConnected: (connected: boolean) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
}

export const useStatusStore = create<StatusState>()((set) => ({
  status: null,
  peers: [],
  satellites: [],
  logs: [],
  config: null,
  connected: false,
  loading: true,
  error: null,
  setStatus: (status) => set({ status, loading: false, error: null }),
  setPeers: (peers) => set({ peers }),
  setSatellites: (satellites) => set({ satellites }),
  setLogs: (logs) => set({ logs }),
  setConfig: (config) => set({ config }),
  setConnected: (connected) => set({ connected }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error, loading: false }),
}));

interface UIState {
  sidebarOpen: boolean;
  theme: 'dark' | 'light';
  toggleSidebar: () => void;
  setTheme: (theme: 'dark' | 'light') => void;
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      sidebarOpen: true,
      theme: 'dark',
      toggleSidebar: () => set((state) => ({ sidebarOpen: !state.sidebarOpen })),
      setTheme: (theme) => set({ theme }),
    }),
    {
      name: 'ui-storage',
    }
  )
);
