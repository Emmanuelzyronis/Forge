// Scene 6 — SUCCEEDED (frames 3300–4200 = 55–70s)
import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, interpolate, Easing, spring } from 'remotion';
import { C, SANS, MONO } from '../palette';
import { data } from '../data';

export const Scene06Success: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const fade = (s: number, e: number) =>
    interpolate(frame, [s * fps, e * fps], [0, 1], { extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic) });

  const scale = spring({ frame, fps, config: { damping: 14, stiffness: 120 }, from: 0.7, to: 1 });

  const totalSecs = Math.round(data.stats.total_duration_secs);
  const recoveryLatency = data.stats.recovery_latency_secs;
  const attemptCount = data.stats.attempt_count;

  return (
    <AbsoluteFill style={{ background: C.bg, fontFamily: SANS, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>

      {/* Success badge */}
      <div style={{
        opacity: fade(0, 0.8),
        transform: `scale(${scale})`,
        background: `${C.succeeded}12`,
        border: `2px solid ${C.succeeded}40`,
        borderRadius: '50%',
        width: 120,
        height: 120,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        marginBottom: 36,
      }}>
        <span style={{ fontSize: 56, color: C.succeeded }}>✓</span>
      </div>

      <div style={{ opacity: fade(0.3, 1.0), textAlign: 'center', marginBottom: 48 }}>
        <div style={{ fontFamily: MONO, fontSize: 42, fontWeight: 700, color: C.succeeded, letterSpacing: '0.04em' }}>
          SUCCEEDED
        </div>
        <div style={{ fontFamily: MONO, fontSize: 16, color: C.muted, marginTop: 10 }}>
          Attempt 2 · worker-02 · terminal_at committed
        </div>
      </div>

      {/* Stats row */}
      <div style={{ opacity: fade(0.8, 1.6), display: 'flex', gap: 48, marginBottom: 40 }}>
        {[
          { label: 'Total time', value: `${totalSecs}s` },
          { label: 'Recovery latency', value: `${recoveryLatency}s` },
          { label: 'Attempts used', value: `${attemptCount} / ${data.stats.max_attempts}` },
          { label: 'Lease duration', value: `${data.stats.lease_duration_secs}s` },
        ].map(({ label, value }) => (
          <div key={label} style={{ textAlign: 'center' }}>
            <div style={{ fontFamily: MONO, fontSize: 26, fontWeight: 700, color: C.text }}>{value}</div>
            <div style={{ fontFamily: MONO, fontSize: 12, color: C.muted, marginTop: 6, letterSpacing: '0.08em' }}>{label}</div>
          </div>
        ))}
      </div>

      {/* Immutable event history note */}
      <div style={{
        opacity: fade(1.5, 2.5),
        background: C.surface,
        border: `1px solid ${C.border}`,
        borderRadius: 6,
        padding: '16px 28px',
        maxWidth: 580,
        textAlign: 'center',
      }}>
        <div style={{ fontFamily: MONO, fontSize: 11, color: C.muted, letterSpacing: '0.1em', marginBottom: 8 }}>IMMUTABLE EVENT LOG (F-INV-010)</div>
        <div style={{ fontFamily: MONO, fontSize: 13, color: C.teal, lineHeight: 1.6 }}>
          Every state write commits its event in the same transaction.
          Neither succeeds without the other.
        </div>
      </div>

    </AbsoluteFill>
  );
};
