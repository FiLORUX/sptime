// API client for SPTime backend
import type {
  SystemStatus,
  PeerStatus,
  Satellite,
  LogEntry,
  LoginResponse,
  Config,
  NTPConfig,
  NTSConfig,
  PTPConfig,
  GPSDOConfig,
  NTPUpstreamPeer,
  NetworkInterface,
  SerialPort,
} from '../types';

const API_BASE = '/api';

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const token = localStorage.getItem('token');

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  if (token) {
    (headers as Record<string, string>)['Authorization'] = `Bearer ${token}`;
  }

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Unknown error' }));
    throw new ApiError(response.status, error.error || 'Request failed');
  }

  // Handle empty responses (204 No Content)
  if (response.status === 204) {
    return {} as T;
  }

  const text = await response.text();
  if (!text) {
    return {} as T;
  }

  return JSON.parse(text);
}

export const api = {
  // Health check
  health: () => request<{ status: string; version: string; time: string }>('/health'),

  // Authentication
  login: (username: string, password: string) =>
    request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  // Status endpoints
  getStatus: () => request<SystemStatus>('/status'),
  getClockStatus: () => request<SystemStatus['clock']>('/status/clock'),
  getNTPStatus: () => request<SystemStatus['ntp']>('/status/ntp'),
  getNTSStatus: () => request<SystemStatus['nts']>('/status/nts'),
  getPTPStatus: () => request<SystemStatus['ptp']>('/status/ptp'),
  getGPSDOStatus: () => request<SystemStatus['gpsdo']>('/status/gpsdo'),

  // Peers and sources
  getPeers: () => request<PeerStatus[]>('/peers'),
  getSatellites: () => request<Satellite[]>('/satellites'),

  // Configuration - Full config
  getConfig: () => request<Config>('/config'),
  updateConfig: (config: Partial<Config>) =>
    request<{ status: string }>('/config', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),
  reloadConfig: () =>
    request<{ status: string }>('/admin/reload-config', { method: 'POST' }),

  // NTP Configuration
  updateNTPConfig: (config: Partial<NTPConfig>) =>
    request<{ status: string }>('/config/ntp', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),

  // NTP Peer CRUD
  getNTPPeers: () => request<NTPUpstreamPeer[]>('/config/ntp/peers'),
  addNTPPeer: (peer: NTPUpstreamPeer) =>
    request<{ status: string; index: number }>('/config/ntp/peers', {
      method: 'POST',
      body: JSON.stringify(peer),
    }),
  updateNTPPeer: (index: number, peer: NTPUpstreamPeer) =>
    request<{ status: string }>(`/config/ntp/peers/${index}`, {
      method: 'PUT',
      body: JSON.stringify(peer),
    }),
  deleteNTPPeer: (index: number) =>
    request<{ status: string }>(`/config/ntp/peers/${index}`, {
      method: 'DELETE',
    }),

  // NTS Configuration
  updateNTSConfig: (config: Partial<NTSConfig>) =>
    request<{ status: string }>('/config/nts', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),

  // PTP Configuration
  updatePTPConfig: (config: Partial<PTPConfig>) =>
    request<{ status: string }>('/config/ptp', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),

  // GPSDO Configuration
  updateGPSDOConfig: (config: Partial<GPSDOConfig>) =>
    request<{ status: string }>('/config/gpsdo', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),

  // System information
  getNetworkInterfaces: () => request<NetworkInterface[]>('/system/interfaces'),
  getSerialPorts: () => request<SerialPort[]>('/system/serial-ports'),

  // Logs
  getLogs: () => request<LogEntry[]>('/logs'),
};

// WebSocket connection for real-time updates
export class StatusWebSocket {
  private ws: WebSocket | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;
  private onStatusUpdate: ((status: SystemStatus) => void) | null = null;
  private onConnectionChange: ((connected: boolean) => void) | null = null;

  connect(
    onStatus: (status: SystemStatus) => void,
    onConnection?: (connected: boolean) => void
  ) {
    this.onStatusUpdate = onStatus;
    this.onConnectionChange = onConnection || null;
    this.establishConnection();
  }

  private establishConnection() {
    const token = localStorage.getItem('token');
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/ws/status${token ? `?token=${token}` : ''}`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      this.reconnectAttempts = 0;
      this.onConnectionChange?.(true);
    };

    this.ws.onmessage = (event) => {
      try {
        const status = JSON.parse(event.data) as SystemStatus;
        this.onStatusUpdate?.(status);
      } catch (e) {
        console.error('Failed to parse WebSocket message:', e);
      }
    };

    this.ws.onclose = () => {
      this.onConnectionChange?.(false);
      this.attemptReconnect();
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }

  private attemptReconnect() {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
      setTimeout(() => this.establishConnection(), delay);
    }
  }

  disconnect() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}

export default api;
