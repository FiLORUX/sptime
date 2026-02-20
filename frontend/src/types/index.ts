// Type definitions for SPTime frontend

export type ClockState = 'INIT' | 'SYNCING' | 'LOCKED' | 'HOLDOVER' | 'FAULT';

export interface ClockStatus {
  state: ClockState;
  current_time: string;
  offset_ns: number;
  offset_stddev_ns: number;
  drift_ppb: number;
  primary_reference: string;
  active_reference: string;
  stratum: number;
  last_sync: string;
  holdover_started?: string;
  holdover_remaining_ns?: number;
}

export interface NTPStatus {
  running: boolean;
  stratum: number;
  ref_id: string;
  root_delay_ms: number;
  root_dispersion_ms: number;
  precision: number;
  peer_count: number;
  last_sync: string;
  offset_us: number;
  jitter_us: number;
  request_count: number;
}

export interface NTSStatus {
  running: boolean;
  ke_port: number;
  active_sessions: number;
  total_ke_requests: number;
  total_ke_success: number;
  total_ke_failures: number;
  cert_expiry: string;
}

export interface PTPStatus {
  running: boolean;
  port_state: string;
  clock_id: string;
  domain: number;
  priority1: number;
  priority2: number;
  clock_class: number;
  clock_accuracy: number;
  profile: string;
  interface: string;
  sync_count: number;
  announce_count: number;
  delay_req_count: number;
  offset_ns: number;
  path_delay_ns: number;
}

export interface GPSDOStatus {
  type: string;
  connected: boolean;
  locked: boolean;
  lock_duration: number;
  satellites: number;
  signal_strength_db: number;
  offset_ns: number;
  latitude?: number;
  longitude?: number;
  altitude_m?: number;
  hdop?: number;
  pps_count: number;
  last_pps: string;
  last_nmea: string;
  firmware?: string;
  serial_number?: string;
  error?: string;
}

export interface Satellite {
  prn: number;
  elevation: number;
  azimuth: number;
  snr: number;
  used: boolean;
}

export interface PeerStatus {
  address: string;
  stratum: number;
  ref_id: string;
  reachable: boolean;
  reach: number;
  offset_us: number;
  delay_us: number;
  jitter_us: number;
  last_poll: string;
  next_poll: string;
  poll_count: number;
  error_count: number;
  selected: boolean;
  nts_enabled: boolean;
}

export interface SystemStatus {
  time: string;
  uptime_seconds: number;
  clock: ClockStatus;
  ntp?: NTPStatus;
  nts?: NTSStatus;
  ptp?: PTPStatus;
  gpsdo?: GPSDOStatus;
}

export interface LogEntry {
  timestamp: string;
  level: string;
  component: string;
  message: string;
  fields?: Record<string, unknown>;
}

export interface User {
  username: string;
  role: 'admin' | 'operator' | 'viewer';
}

export interface LoginResponse {
  token: string;
  expires_in: number;
  user: User;
}

// NTP Upstream Peer configuration
export interface NTPUpstreamPeer {
  address: string;
  prefer: boolean;
  iburst: boolean;
  minpoll: number;
  maxpoll: number;
  nts_enabled: boolean;
}

// NTP Configuration
export interface NTPConfig {
  enabled: boolean;
  port: number;
  interface: string;
  stratum: number;
  poll_interval: string;
  ref_id: string;
  upstream_peers: NTPUpstreamPeer[];
  allowed_networks: string[];
}

// NTS Configuration
export interface NTSConfig {
  enabled: boolean;
  ke_port: number;
  cert_path: string;
  key_path: string;
  ca_path: string;
  cookie_key: string;
  min_tls_version: string;
}

// PTP Configuration
export interface PTPConfig {
  enabled: boolean;
  interface: string;
  domain: number;
  priority1: number;
  priority2: number;
  clock_class: number;
  clock_accuracy: number;
  log_announce_interval: number;
  log_sync_interval: number;
  log_min_delay_req_interval: number;
  transport_mode: 'multicast' | 'unicast';
  delay_mechanism: 'E2E' | 'P2P';
  profile: 'default' | 'smpte2059' | 'telecom' | 'power';
  two_step_flag: boolean;
}

// GPSDO Configuration
export interface GPSDOConfig {
  enabled: boolean;
  type: 'serialpps' | 'network' | 'dummy';
  serial_device: string;
  serial_baud: number;
  pps_device: string;
  network_address: string;
  nmea_sentences: string[];
  poll_interval: string;
}

// Web Configuration
export interface WebConfig {
  port: number;
  bind_address: string;
  tls_enabled: boolean;
  tls_cert: string;
  tls_key: string;
  jwt_secret: string;
  jwt_expiry: string;
  session_timeout: string;
  allowed_origins: string[];
  read_only_mode: boolean;
  rate_limit_rps: number;
}

// Logging Configuration
export interface LoggingConfig {
  level: 'debug' | 'info' | 'warn' | 'error';
  format: 'json' | 'text';
  output: 'stdout' | 'stderr' | 'file';
  file_path: string;
  max_size_mb: number;
  max_backups: number;
  max_age_days: number;
}

// Complete system configuration
export interface Config {
  ntp: NTPConfig;
  nts: NTSConfig;
  ptp: PTPConfig;
  gpsdo: GPSDOConfig;
  web: WebConfig;
  logging: LoggingConfig;
}

// Network interface info
export interface NetworkInterface {
  name: string;
  mac: string;
  addresses: string[];
  flags: string[];
  mtu: number;
}

// Serial port info
export interface SerialPort {
  device: string;
  description: string;
  driver: string;
}

// API response types
export interface ApiResponse<T = void> {
  status: string;
  data?: T;
  error?: string;
}
