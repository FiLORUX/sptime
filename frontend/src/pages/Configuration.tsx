// Configuration page - Complete system settings management
import { useState, useEffect, useCallback } from 'react';
import { useStatusStore, useAuthStore } from '../store';
import { api } from '../api/client';
import type {
  NTPUpstreamPeer,
  NetworkInterface,
  SerialPort,
  NTPConfig,
  NTSConfig,
  PTPConfig,
  GPSDOConfig,
} from '../types';
import {
  Settings,
  Save,
  RefreshCw,
  AlertCircle,
  CheckCircle,
  Plus,
  Trash2,
  Edit3,
  X,
  Clock,
  Shield,
  Radio,
  Satellite,
  ChevronDown,
  ChevronUp,
  Star,
} from 'lucide-react';

// Default values for new peer
const defaultPeer: NTPUpstreamPeer = {
  address: '',
  prefer: false,
  iburst: true,
  minpoll: 4,
  maxpoll: 6,
  nts_enabled: false,
};

// Confirmation dialog component
function ConfirmDialog({
  open,
  title,
  message,
  confirmText = 'Delete',
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  message: string;
  confirmText?: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onCancel} />
      <div className="relative bg-gray-800 border border-gray-700 rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
        <h3 className="text-lg font-semibold text-white mb-2">{title}</h3>
        <p className="text-gray-300 mb-6">{message}</p>
        <div className="flex justify-end gap-3">
          <button
            onClick={onCancel}
            className="btn btn-secondary"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="btn bg-red-600 hover:bg-red-700 text-white"
          >
            {confirmText}
          </button>
        </div>
      </div>
    </div>
  );
}

// Peer editor modal
function PeerEditorModal({
  open,
  peer,
  isNew,
  onSave,
  onCancel,
}: {
  open: boolean;
  peer: NTPUpstreamPeer;
  isNew: boolean;
  onSave: (peer: NTPUpstreamPeer) => void;
  onCancel: () => void;
}) {
  const [formData, setFormData] = useState<NTPUpstreamPeer>(peer);
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    setFormData(peer);
    setErrors({});
  }, [peer, open]);

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};

    if (!formData.address.trim()) {
      newErrors.address = 'Address is required';
    } else if (!/^[\w.-]+:\d+$/.test(formData.address)) {
      newErrors.address = 'Address must be in format host:port (e.g., time.cloudflare.com:123)';
    }

    if (formData.minpoll < 0 || formData.minpoll > 17) {
      newErrors.minpoll = 'Min poll must be between 0 and 17';
    }

    if (formData.maxpoll < 0 || formData.maxpoll > 17) {
      newErrors.maxpoll = 'Max poll must be between 0 and 17';
    }

    if (formData.minpoll > formData.maxpoll) {
      newErrors.maxpoll = 'Max poll must be >= min poll';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (validate()) {
      onSave(formData);
    }
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onCancel} />
      <div className="relative bg-gray-800 border border-gray-700 rounded-lg shadow-xl p-6 max-w-lg w-full mx-4">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-white">
            {isNew ? 'Add NTP Peer' : 'Edit NTP Peer'}
          </h3>
          <button
            onClick={onCancel}
            className="text-gray-400 hover:text-gray-200"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">
              Server Address *
            </label>
            <input
              type="text"
              value={formData.address}
              onChange={(e) => setFormData({ ...formData, address: e.target.value })}
              className={`input w-full ${errors.address ? 'border-red-500' : ''}`}
              placeholder="time.cloudflare.com:123"
            />
            {errors.address && (
              <p className="text-red-400 text-sm mt-1">{errors.address}</p>
            )}
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-1">
                Min Poll (2^n seconds)
              </label>
              <input
                type="number"
                value={formData.minpoll}
                onChange={(e) => setFormData({ ...formData, minpoll: parseInt(e.target.value) || 4 })}
                className={`input w-full ${errors.minpoll ? 'border-red-500' : ''}`}
                min={0}
                max={17}
              />
              {errors.minpoll && (
                <p className="text-red-400 text-sm mt-1">{errors.minpoll}</p>
              )}
              <p className="text-gray-500 text-xs mt-1">
                {formData.minpoll >= 0 && `= ${Math.pow(2, formData.minpoll)} seconds`}
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-300 mb-1">
                Max Poll (2^n seconds)
              </label>
              <input
                type="number"
                value={formData.maxpoll}
                onChange={(e) => setFormData({ ...formData, maxpoll: parseInt(e.target.value) || 6 })}
                className={`input w-full ${errors.maxpoll ? 'border-red-500' : ''}`}
                min={0}
                max={17}
              />
              {errors.maxpoll && (
                <p className="text-red-400 text-sm mt-1">{errors.maxpoll}</p>
              )}
              <p className="text-gray-500 text-xs mt-1">
                {formData.maxpoll >= 0 && `= ${Math.pow(2, formData.maxpoll)} seconds`}
              </p>
            </div>
          </div>

          <div className="space-y-3">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={formData.prefer}
                onChange={(e) => setFormData({ ...formData, prefer: e.target.checked })}
                className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary-500"
              />
              <div>
                <span className="text-gray-200">Prefer this peer</span>
                <p className="text-gray-500 text-xs">Use as primary time source when available</p>
              </div>
            </label>

            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={formData.iburst}
                onChange={(e) => setFormData({ ...formData, iburst: e.target.checked })}
                className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary-500"
              />
              <div>
                <span className="text-gray-200">Initial burst (iburst)</span>
                <p className="text-gray-500 text-xs">Send 8 packets on first poll for faster sync</p>
              </div>
            </label>

            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={formData.nts_enabled}
                onChange={(e) => setFormData({ ...formData, nts_enabled: e.target.checked })}
                className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary-500"
              />
              <div>
                <span className="text-gray-200">Enable NTS</span>
                <p className="text-gray-500 text-xs">Use Network Time Security (RFC 8915)</p>
              </div>
            </label>
          </div>

          <div className="flex justify-end gap-3 pt-4">
            <button
              type="button"
              onClick={onCancel}
              className="btn btn-secondary"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="btn btn-primary"
            >
              {isNew ? 'Add Peer' : 'Save Changes'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

// Collapsible section component
function Section({
  title,
  icon: Icon,
  defaultOpen = true,
  children,
}: {
  title: string;
  icon: React.ElementType;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className="card">
      <button
        onClick={() => setOpen(!open)}
        className="card-header flex items-center justify-between w-full text-left hover:bg-gray-700/50 transition-colors"
      >
        <div className="flex items-center gap-2">
          <Icon className="w-5 h-5" />
          {title}
        </div>
        {open ? (
          <ChevronUp className="w-5 h-5 text-gray-400" />
        ) : (
          <ChevronDown className="w-5 h-5 text-gray-400" />
        )}
      </button>
      {open && <div className="card-body space-y-4">{children}</div>}
    </div>
  );
}

export default function Configuration() {
  const { config, setConfig } = useStatusStore();
  const { user } = useAuthStore();

  // State
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [networkInterfaces, setNetworkInterfaces] = useState<NetworkInterface[]>([]);
  const [serialPorts, setSerialPorts] = useState<SerialPort[]>([]);

  // Peer editor state
  const [peerModalOpen, setPeerModalOpen] = useState(false);
  const [editingPeerIndex, setEditingPeerIndex] = useState<number | null>(null);
  const [editingPeer, setEditingPeer] = useState<NTPUpstreamPeer>(defaultPeer);

  // Confirm dialog state
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deletingPeerIndex, setDeletingPeerIndex] = useState<number | null>(null);

  // Local form state for each section
  const [ntpConfig, setNtpConfig] = useState<NTPConfig | null>(null);
  const [ntsConfig, setNtsConfig] = useState<NTSConfig | null>(null);
  const [ptpConfig, setPtpConfig] = useState<PTPConfig | null>(null);
  const [gpsdoConfig, setGpsdoConfig] = useState<GPSDOConfig | null>(null);

  const isAdmin = user?.role === 'admin';

  // Load data
  const loadConfig = useCallback(async () => {
    setLoading(true);
    try {
      const [configData, interfaces, ports] = await Promise.all([
        api.getConfig(),
        api.getNetworkInterfaces().catch(() => [] as NetworkInterface[]),
        api.getSerialPorts().catch(() => [] as SerialPort[]),
      ]);

      setConfig(configData);
      setNtpConfig(configData.ntp);
      setNtsConfig(configData.nts);
      setPtpConfig(configData.ptp);
      setGpsdoConfig(configData.gpsdo);
      setNetworkInterfaces(interfaces);
      setSerialPorts(ports);
    } catch (err) {
      setMessage({ type: 'error', text: 'Failed to load configuration' });
    } finally {
      setLoading(false);
    }
  }, [setConfig]);

  useEffect(() => {
    loadConfig();
  }, [loadConfig]);

  // Auto-clear message after 5 seconds
  useEffect(() => {
    if (message) {
      const timer = setTimeout(() => setMessage(null), 5000);
      return () => clearTimeout(timer);
    }
  }, [message]);

  // Section save handlers
  const handleSaveNTP = async () => {
    if (!ntpConfig) return;
    setSaving(true);
    setMessage(null);
    try {
      await api.updateNTPConfig(ntpConfig);
      setMessage({ type: 'success', text: 'NTP configuration saved' });
      await loadConfig();
    } catch (err) {
      setMessage({ type: 'error', text: 'Failed to save NTP configuration' });
    } finally {
      setSaving(false);
    }
  };

  const handleSaveNTS = async () => {
    if (!ntsConfig) return;
    setSaving(true);
    setMessage(null);
    try {
      await api.updateNTSConfig(ntsConfig);
      setMessage({ type: 'success', text: 'NTS configuration saved' });
      await loadConfig();
    } catch (err) {
      setMessage({ type: 'error', text: 'Failed to save NTS configuration' });
    } finally {
      setSaving(false);
    }
  };

  const handleSavePTP = async () => {
    if (!ptpConfig) return;
    setSaving(true);
    setMessage(null);
    try {
      await api.updatePTPConfig(ptpConfig);
      setMessage({ type: 'success', text: 'PTP configuration saved' });
      await loadConfig();
    } catch (err) {
      setMessage({ type: 'error', text: 'Failed to save PTP configuration' });
    } finally {
      setSaving(false);
    }
  };

  const handleSaveGPSDO = async () => {
    if (!gpsdoConfig) return;
    setSaving(true);
    setMessage(null);
    try {
      await api.updateGPSDOConfig(gpsdoConfig);
      setMessage({ type: 'success', text: 'GPSDO configuration saved' });
      await loadConfig();
    } catch (err) {
      setMessage({ type: 'error', text: 'Failed to save GPSDO configuration' });
    } finally {
      setSaving(false);
    }
  };

  const handleReload = async () => {
    setLoading(true);
    setMessage(null);
    try {
      await api.reloadConfig();
      await loadConfig();
      setMessage({ type: 'success', text: 'Configuration reloaded from disk' });
    } catch (err) {
      setMessage({ type: 'error', text: 'Failed to reload configuration' });
    } finally {
      setLoading(false);
    }
  };

  // Peer CRUD handlers
  const handleAddPeer = () => {
    setEditingPeerIndex(null);
    setEditingPeer(defaultPeer);
    setPeerModalOpen(true);
  };

  const handleEditPeer = (index: number) => {
    if (!ntpConfig) return;
    setEditingPeerIndex(index);
    setEditingPeer({ ...ntpConfig.upstream_peers[index] });
    setPeerModalOpen(true);
  };

  const handleDeletePeerClick = (index: number) => {
    setDeletingPeerIndex(index);
    setConfirmOpen(true);
  };

  const handleDeletePeerConfirm = async () => {
    if (deletingPeerIndex === null) return;

    setSaving(true);
    setConfirmOpen(false);
    try {
      await api.deleteNTPPeer(deletingPeerIndex);
      setMessage({ type: 'success', text: 'Peer removed' });
      await loadConfig();
    } catch (err) {
      setMessage({ type: 'error', text: 'Failed to remove peer' });
    } finally {
      setSaving(false);
      setDeletingPeerIndex(null);
    }
  };

  const handlePeerSave = async (peer: NTPUpstreamPeer) => {
    setSaving(true);
    setPeerModalOpen(false);
    try {
      if (editingPeerIndex === null) {
        await api.addNTPPeer(peer);
        setMessage({ type: 'success', text: 'Peer added' });
      } else {
        await api.updateNTPPeer(editingPeerIndex, peer);
        setMessage({ type: 'success', text: 'Peer updated' });
      }
      await loadConfig();
    } catch (err) {
      setMessage({ type: 'error', text: `Failed to ${editingPeerIndex === null ? 'add' : 'update'} peer` });
    } finally {
      setSaving(false);
    }
  };

  if (loading && !config) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-500" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Configuration</h1>
          <p className="text-gray-400">Manage system settings</p>
        </div>
        <button
          onClick={handleReload}
          disabled={loading || !isAdmin}
          className="btn btn-secondary flex items-center gap-2"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          Reload from Disk
        </button>
      </div>

      {/* Status message */}
      {message && (
        <div
          className={`flex items-center gap-2 p-4 rounded-lg ${
            message.type === 'success'
              ? 'bg-green-900/50 border border-green-700 text-green-300'
              : 'bg-red-900/50 border border-red-700 text-red-300'
          }`}
        >
          {message.type === 'success' ? (
            <CheckCircle className="w-5 h-5" />
          ) : (
            <AlertCircle className="w-5 h-5" />
          )}
          {message.text}
        </div>
      )}

      {!isAdmin && (
        <div className="bg-yellow-900/30 border border-yellow-700 rounded-lg p-4 text-yellow-300">
          <p className="flex items-center gap-2">
            <AlertCircle className="w-5 h-5" />
            You need admin privileges to modify configuration
          </p>
        </div>
      )}

      {/* NTP Configuration */}
      <Section title="NTP Server Configuration" icon={Clock}>
        {ntpConfig && (
          <>
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={ntpConfig.enabled}
                  onChange={(e) => setNtpConfig({ ...ntpConfig, enabled: e.target.checked })}
                  disabled={!isAdmin}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary-500 focus:ring-primary-500"
                />
                <span className="text-gray-200">Enable NTP Server</span>
              </label>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Port</label>
                <input
                  type="number"
                  value={ntpConfig.port}
                  onChange={(e) => setNtpConfig({ ...ntpConfig, port: parseInt(e.target.value) || 123 })}
                  disabled={!isAdmin}
                  className="input w-full"
                  min={1}
                  max={65535}
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Stratum</label>
                <input
                  type="number"
                  value={ntpConfig.stratum}
                  onChange={(e) => setNtpConfig({ ...ntpConfig, stratum: parseInt(e.target.value) || 2 })}
                  disabled={!isAdmin}
                  className="input w-full"
                  min={1}
                  max={15}
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Reference ID</label>
                <input
                  type="text"
                  value={ntpConfig.ref_id}
                  onChange={(e) => setNtpConfig({ ...ntpConfig, ref_id: e.target.value.toUpperCase().slice(0, 4) })}
                  disabled={!isAdmin}
                  className="input w-full"
                  maxLength={4}
                  placeholder="LOCL"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Poll Interval</label>
                <select
                  value={ntpConfig.poll_interval}
                  onChange={(e) => setNtpConfig({ ...ntpConfig, poll_interval: e.target.value })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  <option value="16s">16 seconds</option>
                  <option value="32s">32 seconds</option>
                  <option value="64s">64 seconds</option>
                  <option value="128s">128 seconds</option>
                  <option value="256s">256 seconds</option>
                </select>
              </div>
            </div>

            {/* Upstream Peers */}
            <div className="border-t border-gray-700 pt-4 mt-4">
              <div className="flex items-center justify-between mb-3">
                <h4 className="text-sm font-medium text-gray-200">Upstream NTP Peers</h4>
                {isAdmin && (
                  <button
                    onClick={handleAddPeer}
                    className="btn btn-sm btn-primary flex items-center gap-1"
                  >
                    <Plus className="w-4 h-4" />
                    Add Peer
                  </button>
                )}
              </div>

              {ntpConfig.upstream_peers.length === 0 ? (
                <p className="text-gray-500 text-sm italic">No upstream peers configured</p>
              ) : (
                <div className="space-y-2">
                  {ntpConfig.upstream_peers.map((peer, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between p-3 bg-gray-900/50 rounded-lg border border-gray-700"
                    >
                      <div className="flex items-center gap-3">
                        {peer.prefer && (
                          <span title="Preferred peer">
                            <Star className="w-4 h-4 text-yellow-400" />
                          </span>
                        )}
                        <div>
                          <div className="text-gray-200 font-mono">{peer.address}</div>
                          <div className="text-gray-500 text-xs flex items-center gap-2">
                            <span>Poll: 2^{peer.minpoll} - 2^{peer.maxpoll}s</span>
                            {peer.iburst && <span className="text-primary-400">iburst</span>}
                            {peer.nts_enabled && (
                              <span className="text-green-400 flex items-center gap-1">
                                <Shield className="w-3 h-3" /> NTS
                              </span>
                            )}
                          </div>
                        </div>
                      </div>

                      {isAdmin && (
                        <div className="flex items-center gap-2">
                          <button
                            onClick={() => handleEditPeer(index)}
                            className="p-1.5 text-gray-400 hover:text-gray-200 hover:bg-gray-700 rounded"
                            title="Edit peer"
                          >
                            <Edit3 className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => handleDeletePeerClick(index)}
                            className="p-1.5 text-gray-400 hover:text-red-400 hover:bg-gray-700 rounded"
                            title="Remove peer"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {isAdmin && (
              <div className="flex justify-end pt-4 border-t border-gray-700">
                <button
                  onClick={handleSaveNTP}
                  disabled={saving}
                  className="btn btn-primary flex items-center gap-2"
                >
                  <Save className="w-4 h-4" />
                  {saving ? 'Saving...' : 'Save NTP Settings'}
                </button>
              </div>
            )}
          </>
        )}
      </Section>

      {/* NTS Configuration */}
      <Section title="NTS (Network Time Security) Configuration" icon={Shield} defaultOpen={false}>
        {ntsConfig && (
          <>
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={ntsConfig.enabled}
                  onChange={(e) => setNtsConfig({ ...ntsConfig, enabled: e.target.checked })}
                  disabled={!isAdmin}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary-500 focus:ring-primary-500"
                />
                <span className="text-gray-200">Enable NTS Server</span>
              </label>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">NTS-KE Port</label>
                <input
                  type="number"
                  value={ntsConfig.ke_port}
                  onChange={(e) => setNtsConfig({ ...ntsConfig, ke_port: parseInt(e.target.value) || 4460 })}
                  disabled={!isAdmin}
                  className="input w-full"
                  min={1}
                  max={65535}
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Min TLS Version</label>
                <select
                  value={ntsConfig.min_tls_version}
                  onChange={(e) => setNtsConfig({ ...ntsConfig, min_tls_version: e.target.value })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  <option value="1.2">TLS 1.2</option>
                  <option value="1.3">TLS 1.3 (Recommended)</option>
                </select>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Certificate Path</label>
                <input
                  type="text"
                  value={ntsConfig.cert_path}
                  onChange={(e) => setNtsConfig({ ...ntsConfig, cert_path: e.target.value })}
                  disabled={!isAdmin}
                  className="input w-full"
                  placeholder="/etc/sptime/certs/server.crt"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Key Path</label>
                <input
                  type="text"
                  value={ntsConfig.key_path}
                  onChange={(e) => setNtsConfig({ ...ntsConfig, key_path: e.target.value })}
                  disabled={!isAdmin}
                  className="input w-full"
                  placeholder="/etc/sptime/certs/server.key"
                />
              </div>
            </div>

            {ntsConfig.enabled && (!ntsConfig.cert_path || !ntsConfig.key_path) && (
              <div className="bg-yellow-900/30 border border-yellow-700 rounded-lg p-3 text-yellow-300 text-sm">
                <AlertCircle className="w-4 h-4 inline mr-2" />
                NTS requires valid TLS certificate and key paths
              </div>
            )}

            {isAdmin && (
              <div className="flex justify-end pt-4 border-t border-gray-700">
                <button
                  onClick={handleSaveNTS}
                  disabled={saving}
                  className="btn btn-primary flex items-center gap-2"
                >
                  <Save className="w-4 h-4" />
                  {saving ? 'Saving...' : 'Save NTS Settings'}
                </button>
              </div>
            )}
          </>
        )}
      </Section>

      {/* PTP Configuration */}
      <Section title="PTP Grandmaster Configuration" icon={Radio} defaultOpen={false}>
        {ptpConfig && (
          <>
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={ptpConfig.enabled}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, enabled: e.target.checked })}
                  disabled={!isAdmin}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary-500 focus:ring-primary-500"
                />
                <span className="text-gray-200">Enable PTP Grandmaster</span>
              </label>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Network Interface</label>
                <select
                  value={ptpConfig.interface}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, interface: e.target.value })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  {networkInterfaces.length === 0 ? (
                    <option value={ptpConfig.interface}>{ptpConfig.interface}</option>
                  ) : (
                    networkInterfaces.map((iface) => (
                      <option key={iface.name} value={iface.name}>
                        {iface.name} ({iface.addresses.join(', ') || 'no IP'})
                      </option>
                    ))
                  )}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Domain</label>
                <input
                  type="number"
                  value={ptpConfig.domain}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, domain: parseInt(e.target.value) || 0 })}
                  disabled={!isAdmin}
                  className="input w-full"
                  min={0}
                  max={127}
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Priority 1</label>
                <input
                  type="number"
                  value={ptpConfig.priority1}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, priority1: parseInt(e.target.value) || 128 })}
                  disabled={!isAdmin}
                  className="input w-full"
                  min={0}
                  max={255}
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Priority 2</label>
                <input
                  type="number"
                  value={ptpConfig.priority2}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, priority2: parseInt(e.target.value) || 128 })}
                  disabled={!isAdmin}
                  className="input w-full"
                  min={0}
                  max={255}
                />
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Profile</label>
                <select
                  value={ptpConfig.profile}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, profile: e.target.value as PTPConfig['profile'] })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  <option value="default">Default (IEEE 1588)</option>
                  <option value="smpte2059">SMPTE 2059 (Broadcast)</option>
                  <option value="telecom">ITU-T Telecom</option>
                  <option value="power">IEEE C37.238 (Power)</option>
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Transport Mode</label>
                <select
                  value={ptpConfig.transport_mode}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, transport_mode: e.target.value as PTPConfig['transport_mode'] })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  <option value="multicast">Multicast</option>
                  <option value="unicast">Unicast</option>
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Delay Mechanism</label>
                <select
                  value={ptpConfig.delay_mechanism}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, delay_mechanism: e.target.value as PTPConfig['delay_mechanism'] })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  <option value="E2E">End-to-End (E2E)</option>
                  <option value="P2P">Peer-to-Peer (P2P)</option>
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Clock Class</label>
                <select
                  value={ptpConfig.clock_class}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, clock_class: parseInt(e.target.value) })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  <option value="6">6 - Primary Reference (GPS)</option>
                  <option value="7">7 - Primary Holdover</option>
                  <option value="52">52 - PTP Traceable (ARB)</option>
                  <option value="187">187 - ARB Holdover</option>
                  <option value="248">248 - Default</option>
                </select>
              </div>
            </div>

            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={ptpConfig.two_step_flag}
                  onChange={(e) => setPtpConfig({ ...ptpConfig, two_step_flag: e.target.checked })}
                  disabled={!isAdmin}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary-500 focus:ring-primary-500"
                />
                <span className="text-gray-200">Two-step mode (recommended for software timestamps)</span>
              </label>
            </div>

            {ptpConfig.enabled && !ptpConfig.interface && (
              <div className="bg-yellow-900/30 border border-yellow-700 rounded-lg p-3 text-yellow-300 text-sm">
                <AlertCircle className="w-4 h-4 inline mr-2" />
                PTP requires a network interface to be selected
              </div>
            )}

            {isAdmin && (
              <div className="flex justify-end pt-4 border-t border-gray-700">
                <button
                  onClick={handleSavePTP}
                  disabled={saving}
                  className="btn btn-primary flex items-center gap-2"
                >
                  <Save className="w-4 h-4" />
                  {saving ? 'Saving...' : 'Save PTP Settings'}
                </button>
              </div>
            )}
          </>
        )}
      </Section>

      {/* GPSDO Configuration */}
      <Section title="GPSDO Configuration" icon={Satellite} defaultOpen={false}>
        {gpsdoConfig && (
          <>
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={gpsdoConfig.enabled}
                  onChange={(e) => setGpsdoConfig({ ...gpsdoConfig, enabled: e.target.checked })}
                  disabled={!isAdmin}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary-500 focus:ring-primary-500"
                />
                <span className="text-gray-200">Enable GPSDO</span>
              </label>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">GPSDO Type</label>
                <select
                  value={gpsdoConfig.type}
                  onChange={(e) => setGpsdoConfig({ ...gpsdoConfig, type: e.target.value as GPSDOConfig['type'] })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  <option value="dummy">Dummy (Testing)</option>
                  <option value="serialpps">Serial + PPS</option>
                  <option value="network">Network GPS</option>
                </select>
              </div>

              {gpsdoConfig.type === 'serialpps' && (
                <>
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-1">Serial Device</label>
                    <select
                      value={gpsdoConfig.serial_device}
                      onChange={(e) => setGpsdoConfig({ ...gpsdoConfig, serial_device: e.target.value })}
                      disabled={!isAdmin}
                      className="input w-full"
                    >
                      {serialPorts.length === 0 ? (
                        <option value={gpsdoConfig.serial_device}>{gpsdoConfig.serial_device}</option>
                      ) : (
                        serialPorts.map((port) => (
                          <option key={port.device} value={port.device}>
                            {port.device} {port.description && `(${port.description})`}
                          </option>
                        ))
                      )}
                      <option value="/dev/ttyUSB0">/dev/ttyUSB0</option>
                      <option value="/dev/ttyUSB1">/dev/ttyUSB1</option>
                      <option value="/dev/ttyACM0">/dev/ttyACM0</option>
                      <option value="/dev/ttyS0">/dev/ttyS0</option>
                    </select>
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-1">Serial Baud Rate</label>
                    <select
                      value={gpsdoConfig.serial_baud}
                      onChange={(e) => setGpsdoConfig({ ...gpsdoConfig, serial_baud: parseInt(e.target.value) })}
                      disabled={!isAdmin}
                      className="input w-full"
                    >
                      <option value="4800">4800</option>
                      <option value="9600">9600</option>
                      <option value="19200">19200</option>
                      <option value="38400">38400</option>
                      <option value="57600">57600</option>
                      <option value="115200">115200</option>
                    </select>
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-1">PPS Device</label>
                    <select
                      value={gpsdoConfig.pps_device}
                      onChange={(e) => setGpsdoConfig({ ...gpsdoConfig, pps_device: e.target.value })}
                      disabled={!isAdmin}
                      className="input w-full"
                    >
                      <option value="/dev/pps0">/dev/pps0</option>
                      <option value="/dev/pps1">/dev/pps1</option>
                      <option value="/dev/pps2">/dev/pps2</option>
                    </select>
                  </div>
                </>
              )}

              {gpsdoConfig.type === 'network' && (
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1">Network Address</label>
                  <input
                    type="text"
                    value={gpsdoConfig.network_address}
                    onChange={(e) => setGpsdoConfig({ ...gpsdoConfig, network_address: e.target.value })}
                    disabled={!isAdmin}
                    className="input w-full"
                    placeholder="192.168.1.100:5000"
                  />
                </div>
              )}

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Poll Interval</label>
                <select
                  value={gpsdoConfig.poll_interval}
                  onChange={(e) => setGpsdoConfig({ ...gpsdoConfig, poll_interval: e.target.value })}
                  disabled={!isAdmin}
                  className="input w-full"
                >
                  <option value="100ms">100ms</option>
                  <option value="250ms">250ms</option>
                  <option value="500ms">500ms</option>
                  <option value="1s">1 second</option>
                  <option value="2s">2 seconds</option>
                </select>
              </div>
            </div>

            {gpsdoConfig.type === 'serialpps' && (
              <div className="bg-blue-900/30 border border-blue-700 rounded-lg p-3 text-blue-300 text-sm">
                <Settings className="w-4 h-4 inline mr-2" />
                For Serial+PPS mode, ensure the GPS receiver is connected via USB/Serial and the PPS signal is wired to the system's PPS input.
              </div>
            )}

            {isAdmin && (
              <div className="flex justify-end pt-4 border-t border-gray-700">
                <button
                  onClick={handleSaveGPSDO}
                  disabled={saving}
                  className="btn btn-primary flex items-center gap-2"
                >
                  <Save className="w-4 h-4" />
                  {saving ? 'Saving...' : 'Save GPSDO Settings'}
                </button>
              </div>
            )}
          </>
        )}
      </Section>

      {/* Modals */}
      <PeerEditorModal
        open={peerModalOpen}
        peer={editingPeer}
        isNew={editingPeerIndex === null}
        onSave={handlePeerSave}
        onCancel={() => setPeerModalOpen(false)}
      />

      <ConfirmDialog
        open={confirmOpen}
        title="Remove NTP Peer"
        message={`Are you sure you want to remove this peer? This action cannot be undone.`}
        confirmText="Remove"
        onConfirm={handleDeletePeerConfirm}
        onCancel={() => {
          setConfirmOpen(false);
          setDeletingPeerIndex(null);
        }}
      />
    </div>
  );
}
