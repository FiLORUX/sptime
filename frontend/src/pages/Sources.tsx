// Sources page - NTP peers, GPSDO, and satellite information
import { useStatusStore } from '../store';
import { Radio, Satellite, Signal, Check, X } from 'lucide-react';
import clsx from 'clsx';

function formatTime(timestamp: string): string {
  if (!timestamp) return 'Never';
  const date = new Date(timestamp);
  return date.toLocaleTimeString('en-US', { hour12: false });
}

export default function Sources() {
  const { status, peers, satellites } = useStatusStore();

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold text-white">Time Sources</h1>
        <p className="text-gray-400">NTP peers, GPSDO status, and satellite information</p>
      </div>

      {/* NTP Peers */}
      <div className="card">
        <div className="card-header flex items-center gap-2">
          <Radio className="w-5 h-5" />
          NTP Upstream Peers
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-700">
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Status</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Address</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Stratum</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Ref ID</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-gray-400">Offset</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-gray-400">Delay</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-gray-400">Jitter</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Reach</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Last Poll</th>
              </tr>
            </thead>
            <tbody>
              {peers.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-4 py-8 text-center text-gray-400">
                    No NTP peers configured
                  </td>
                </tr>
              ) : (
                peers.map((peer) => (
                  <tr
                    key={peer.address}
                    className={clsx(
                      'border-b border-gray-700/50 hover:bg-gray-700/30',
                      peer.selected && 'bg-primary-900/20'
                    )}
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        {peer.reachable ? (
                          <Check className="w-4 h-4 text-green-400" />
                        ) : (
                          <X className="w-4 h-4 text-red-400" />
                        )}
                        {peer.selected && (
                          <span className="text-xs bg-primary-600 px-1.5 py-0.5 rounded">
                            SYS
                          </span>
                        )}
                        {peer.nts_enabled && (
                          <span className="text-xs bg-purple-600 px-1.5 py-0.5 rounded">
                            NTS
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3 font-mono text-sm text-white">
                      {peer.address}
                    </td>
                    <td className="px-4 py-3 text-white">{peer.stratum}</td>
                    <td className="px-4 py-3 font-mono text-sm text-gray-300">
                      {peer.ref_id}
                    </td>
                    <td className="px-4 py-3 text-right font-mono text-sm text-white">
                      {peer.offset_us.toFixed(3)} µs
                    </td>
                    <td className="px-4 py-3 text-right font-mono text-sm text-gray-300">
                      {peer.delay_us.toFixed(3)} µs
                    </td>
                    <td className="px-4 py-3 text-right font-mono text-sm text-gray-300">
                      {peer.jitter_us.toFixed(3)} µs
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-0.5">
                        {[...Array(8)].map((_, i) => (
                          <div
                            key={i}
                            className={clsx(
                              'w-2 h-4 rounded-sm',
                              peer.reach & (1 << (7 - i))
                                ? 'bg-green-500'
                                : 'bg-gray-600'
                            )}
                          />
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-300">
                      {formatTime(peer.last_poll)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* GPSDO Status */}
      {status?.gpsdo && (
        <div className="card">
          <div className="card-header flex items-center gap-2">
            <Satellite className="w-5 h-5" />
            GPSDO Status
          </div>
          <div className="card-body">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
              <div>
                <p className="text-sm text-gray-400">Lock Status</p>
                <div className="mt-1 flex items-center gap-2">
                  {status.gpsdo.locked ? (
                    <>
                      <div className="w-3 h-3 rounded-full bg-green-500" />
                      <span className="text-green-400 font-medium">Locked</span>
                    </>
                  ) : (
                    <>
                      <div className="w-3 h-3 rounded-full bg-yellow-500 animate-pulse" />
                      <span className="text-yellow-400 font-medium">Acquiring</span>
                    </>
                  )}
                </div>
              </div>

              <div>
                <p className="text-sm text-gray-400">Satellites in View</p>
                <p className="mt-1 text-2xl font-bold text-white">
                  {status.gpsdo.satellites}
                </p>
              </div>

              <div>
                <p className="text-sm text-gray-400">Signal Strength</p>
                <p className="mt-1 text-2xl font-bold text-white">
                  {status.gpsdo.signal_strength_db.toFixed(1)} dB
                </p>
              </div>

              <div>
                <p className="text-sm text-gray-400">PPS Count</p>
                <p className="mt-1 text-2xl font-bold font-mono text-white">
                  {status.gpsdo.pps_count.toLocaleString()}
                </p>
              </div>

              {status.gpsdo.latitude !== undefined && (
                <div>
                  <p className="text-sm text-gray-400">Latitude</p>
                  <p className="mt-1 text-lg font-mono text-white">
                    {status.gpsdo.latitude.toFixed(6)}°
                  </p>
                </div>
              )}

              {status.gpsdo.longitude !== undefined && (
                <div>
                  <p className="text-sm text-gray-400">Longitude</p>
                  <p className="mt-1 text-lg font-mono text-white">
                    {status.gpsdo.longitude.toFixed(6)}°
                  </p>
                </div>
              )}

              {status.gpsdo.altitude_m !== undefined && (
                <div>
                  <p className="text-sm text-gray-400">Altitude</p>
                  <p className="mt-1 text-lg font-mono text-white">
                    {status.gpsdo.altitude_m.toFixed(1)} m
                  </p>
                </div>
              )}

              {status.gpsdo.hdop !== undefined && (
                <div>
                  <p className="text-sm text-gray-400">HDOP</p>
                  <p className="mt-1 text-lg font-mono text-white">
                    {status.gpsdo.hdop.toFixed(2)}
                  </p>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Satellite List */}
      {satellites.length > 0 && (
        <div className="card">
          <div className="card-header flex items-center gap-2">
            <Signal className="w-5 h-5" />
            GPS Satellites
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-700">
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">PRN</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">Used</th>
                  <th className="px-4 py-3 text-right text-sm font-medium text-gray-400">Elevation</th>
                  <th className="px-4 py-3 text-right text-sm font-medium text-gray-400">Azimuth</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-400">SNR</th>
                </tr>
              </thead>
              <tbody>
                {satellites.map((sat) => (
                  <tr
                    key={sat.prn}
                    className={clsx(
                      'border-b border-gray-700/50',
                      sat.used && 'bg-green-900/10'
                    )}
                  >
                    <td className="px-4 py-3 font-mono text-white">
                      {sat.prn < 32 ? `G${sat.prn}` : `R${sat.prn - 32}`}
                    </td>
                    <td className="px-4 py-3">
                      {sat.used ? (
                        <Check className="w-4 h-4 text-green-400" />
                      ) : (
                        <X className="w-4 h-4 text-gray-500" />
                      )}
                    </td>
                    <td className="px-4 py-3 text-right font-mono text-gray-300">
                      {sat.elevation}°
                    </td>
                    <td className="px-4 py-3 text-right font-mono text-gray-300">
                      {sat.azimuth}°
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <div className="flex-1 h-2 bg-gray-700 rounded-full overflow-hidden max-w-[100px]">
                          <div
                            className={clsx(
                              'h-full rounded-full',
                              sat.snr > 40 ? 'bg-green-500' :
                              sat.snr > 30 ? 'bg-yellow-500' : 'bg-red-500'
                            )}
                            style={{ width: `${Math.min(sat.snr * 2, 100)}%` }}
                          />
                        </div>
                        <span className="font-mono text-sm text-gray-300 w-12">
                          {sat.snr.toFixed(0)} dB
                        </span>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
