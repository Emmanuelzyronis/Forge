// Scene 7 — Live evidence from the run (frames 4200–5400 = 70–90s)
import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, interpolate, Easing } from 'remotion';
import { C, SANS, MONO, stateColor } from '../palette';
import { data } from '../data';

export const Scene07Evidence: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const fade = (s: number, e: number) =>
    interpolate(frame, [s * fps, e * fps], [0, 1], { extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic) });

  const titleFade = fade(0, 0.6);

  // Stagger each timeline row
  const rowFade = (i: number) =>
    interpolate(frame, [(0.3 + i * 0.18) * fps, (0.8 + i * 0.18) * fps], [0, 1], { extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic) });

  const entries = data.timeline.slice(0, 8);

  const footerFade = fade(entries.length * 0.18 + 1.5, entries.length * 0.18 + 2.5);

  return (
    <AbsoluteFill style={{ background: C.bg, fontFamily: SANS, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>

      <div style={{ opacity: titleFade, marginBottom: 36, textAlign: 'center' }}>
        <div style={{ fontFamily: MONO, fontSize: 12, letterSpacing: '0.2em', color: C.muted, textTransform: 'uppercase', marginBottom: 8 }}>
          Live Evidence — from this run
        </div>
        <div style={{ fontFamily: MONO, fontSize: 11, color: C.faint }}>
          run_id: {data.run_id}
        </div>
      </div>

      {/* Event log table */}
      <div style={{ width: 860, background: C.surface, border: `1px solid ${C.border}`, borderRadius: 8, overflow: 'hidden' }}>

        {/* Header */}
        <div style={{ display: 'grid', gridTemplateColumns: '90px 1fr 140px 140px', gap: 0, padding: '12px 24px', borderBottom: `1px solid ${C.border}`, background: C.bg }}>
          {['T (s)', 'Event', 'From', 'To / Worker'].map(h => (
            <span key={h} style={{ fontFamily: MONO, fontSize: 11, color: C.muted, letterSpacing: '0.1em', textTransform: 'uppercase' }}>{h}</span>
          ))}
        </div>

        {entries.map((e, i) => {
          const color = e.is_crash ? C.red : e.is_recovery ? C.orange : stateColor(e.to_state ?? e.event_type);
          return (
            <div
              key={i}
              style={{
                opacity: rowFade(i),
                display: 'grid',
                gridTemplateColumns: '90px 1fr 140px 140px',
                gap: 0,
                padding: '10px 24px',
                borderBottom: `1px solid ${C.border}`,
                background: e.is_crash ? `${C.red}08` : e.is_recovery ? `${C.orange}08` : 'transparent',
              }}
            >
              <span style={{ fontFamily: MONO, fontSize: 13, color: C.muted }}>
                {e.elapsed_secs.toFixed(1)}
              </span>
              <span style={{ fontFamily: MONO, fontSize: 13, fontWeight: e.is_crash ? 700 : 400, color }}>
                {e.event_type}
              </span>
              <span style={{ fontFamily: MONO, fontSize: 12, color: C.muted }}>
                {e.from_state || '—'}
              </span>
              <span style={{ fontFamily: MONO, fontSize: 12, color: e.worker_name ? C.text : C.muted }}>
                {e.to_state || e.worker_name || '—'}
              </span>
            </div>
          );
        })}
      </div>

      {/* Footer callout */}
      <div style={{ opacity: footerFade, marginTop: 28, textAlign: 'center', maxWidth: 660 }}>
        <p style={{ fontFamily: MONO, fontSize: 14, color: C.muted, lineHeight: 1.6, margin: 0 }}>
          Every event written atomically with its state change.
          Job recovered without operator intervention.
          Correct by construction.
        </p>
      </div>

    </AbsoluteFill>
  );
};
