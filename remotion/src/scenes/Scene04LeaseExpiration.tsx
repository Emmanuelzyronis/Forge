// Scene 4 — Lease expiration countdown (frames 1500–2220 = 25–37s)
import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, interpolate, Easing } from 'remotion';
import { C, SANS, MONO } from '../palette';
import { data } from '../data';

export const Scene04LeaseExpiration: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const leaseSecs = data.stats.lease_duration_secs;
  const sweepSecs = data.stats.recovery_interval_secs;

  // Animate countdown: 30s lease → 0, then sweep window
  const countdownDuration = fps * 6;
  const countdownProgress = Math.min(frame / countdownDuration, 1);
  const remainingSecs = Math.max(0, Math.round(leaseSecs * (1 - countdownProgress)));

  const leaseBarWidth = (1 - countdownProgress) * 100;
  const leaseExpired = countdownProgress >= 1;

  const sweepFade = leaseExpired
    ? interpolate(frame, [countdownDuration, countdownDuration + fps * 0.8], [0, 1], { extrapolateRight: 'clamp' })
    : 0;

  const sqlFade = interpolate(frame, [fps * 0.5, fps * 1.5], [0, 1], { extrapolateRight: 'clamp' });
  const titleFade = interpolate(frame, [0, fps * 0.6], [0, 1], { extrapolateRight: 'clamp' });

  const leaseColor = leaseBarWidth > 40 ? C.green : leaseBarWidth > 15 ? C.yellow : C.red;

  return (
    <AbsoluteFill style={{ background: C.bg, fontFamily: SANS, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>

      <div style={{ opacity: titleFade, marginBottom: 52, textAlign: 'center' }}>
        <span style={{ fontFamily: MONO, fontSize: 12, letterSpacing: '0.2em', color: C.muted, textTransform: 'uppercase' }}>
          Lease Expiration
        </span>
      </div>

      {/* Lease countdown */}
      <div style={{ width: 560, marginBottom: 32 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 10 }}>
          <span style={{ fontFamily: MONO, fontSize: 13, color: C.muted, letterSpacing: '0.08em' }}>LEASE REMAINING</span>
          <span style={{ fontFamily: MONO, fontSize: 22, fontWeight: 700, color: leaseExpired ? C.red : leaseColor }}>
            {leaseExpired ? 'EXPIRED' : `${remainingSecs}s`}
          </span>
        </div>
        <div style={{ height: 8, background: C.surface, borderRadius: 4, overflow: 'hidden', border: `1px solid ${C.border}` }}>
          <div style={{
            height: '100%',
            width: `${leaseBarWidth}%`,
            background: leaseExpired ? C.red : leaseColor,
            borderRadius: 4,
            transition: 'background 0.3s',
          }} />
        </div>
        <div style={{ fontFamily: MONO, fontSize: 11, color: C.muted, marginTop: 8, textAlign: 'center' }}>
          lease_expires_at — no heartbeat renewal since worker-01 crashed
        </div>
      </div>

      {/* SQL snippet */}
      <div style={{ opacity: sqlFade, background: C.surface, border: `1px solid ${C.border}`, borderRadius: 6, padding: '20px 28px', maxWidth: 660 }}>
        <div style={{ fontFamily: MONO, fontSize: 11, color: C.muted, marginBottom: 10, letterSpacing: '0.1em' }}>HEARTBEAT SQL (never called after crash)</div>
        <pre style={{ fontFamily: MONO, fontSize: 13, color: C.teal, margin: 0, lineHeight: 1.6 }}>
{`UPDATE jobs
  SET lease_expires_at = NOW() + INTERVAL '${leaseSecs} seconds'
  WHERE id = $1
    AND lease_token = $2
    AND state IN ('CLAIMED', 'RUNNING')`}
        </pre>
      </div>

      {/* Recovery sweep */}
      {leaseExpired && (
        <div style={{ opacity: sweepFade, marginTop: 36, textAlign: 'center', maxWidth: 620 }}>
          <div style={{ fontFamily: MONO, fontSize: 16, color: C.orange, fontWeight: 700, letterSpacing: '0.08em' }}>
            RECOVERY SCHEDULER SWEEP
          </div>
          <div style={{ fontFamily: MONO, fontSize: 13, color: C.muted, marginTop: 10, lineHeight: 1.7 }}>
            SELECT id FROM jobs WHERE state IN ('CLAIMED','RUNNING') AND lease_expires_at &lt; NOW()
          </div>
          <div style={{ fontFamily: MONO, fontSize: 12, color: C.muted, marginTop: 6 }}>
            sweeps every {sweepSecs}s · SELECT FOR UPDATE prevents double-recovery
          </div>
        </div>
      )}

    </AbsoluteFill>
  );
};
