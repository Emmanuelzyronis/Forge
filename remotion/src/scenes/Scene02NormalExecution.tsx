// Scene 2 — Normal execution happy path (frames 300–780 = 5–13s)
import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, interpolate, Easing } from 'remotion';
import { C, SANS, MONO, stateColor } from '../palette';

interface StateBoxProps {
  label: string;
  visible: number; // 0–1
  active?: boolean;
  color: string;
}

const StateBox: React.FC<StateBoxProps> = ({ label, visible, active, color }) => (
  <div style={{
    opacity: visible,
    transform: `scale(${interpolate(visible, [0, 1], [0.85, 1])})`,
    background: active ? `${color}18` : `${C.surface}`,
    border: `1.5px solid ${active ? color : C.border}`,
    borderRadius: 6,
    padding: '18px 32px',
    minWidth: 160,
    textAlign: 'center',
    transition: 'all 0.3s',
  }}>
    <span style={{ fontFamily: MONO, fontSize: 15, fontWeight: 700, color: active ? color : C.muted, letterSpacing: '0.12em' }}>
      {label}
    </span>
  </div>
);

const Arrow: React.FC<{ visible: number; label: string }> = ({ visible, label }) => (
  <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', opacity: visible, margin: '0 8px' }}>
    <span style={{ fontFamily: MONO, fontSize: 22, color: C.faint }}>→</span>
    <span style={{ fontFamily: MONO, fontSize: 11, color: C.muted, marginTop: 4, letterSpacing: '0.08em' }}>{label}</span>
  </div>
);

export const Scene02NormalExecution: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const fade = (start: number, end: number) =>
    interpolate(frame, [start, end], [0, 1], { extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic) });

  const titleFade  = fade(0, fps * 0.7);
  const queuedFade = fade(fps * 0.5, fps * 1.2);
  const arrow1     = fade(fps * 1.2, fps * 1.8);
  const claimedFade = fade(fps * 1.4, fps * 2.0);
  const arrow2     = fade(fps * 2.0, fps * 2.5);
  const runningFade = fade(fps * 2.2, fps * 2.8);
  const arrow3     = fade(fps * 2.8, fps * 3.3);
  const succeededFade = fade(fps * 3.0, fps * 3.6);

  const step = Math.floor(frame / (fps * 0.5));

  return (
    <AbsoluteFill style={{ background: C.bg, fontFamily: SANS, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>

      <div style={{ opacity: titleFade, marginBottom: 56 }}>
        <span style={{ fontFamily: MONO, fontSize: 12, letterSpacing: '0.2em', color: C.muted, textTransform: 'uppercase' }}>
          Happy Path — Normal Execution
        </span>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
        <StateBox label="QUEUED"    visible={queuedFade}    active={step === 1}  color={C.queued} />
        <Arrow label="ClaimNext"    visible={arrow1} />
        <StateBox label="CLAIMED"   visible={claimedFade}   active={step === 2}  color={C.claimed} />
        <Arrow label="Start"        visible={arrow2} />
        <StateBox label="RUNNING"   visible={runningFade}   active={step >= 3}   color={C.running} />
        <Arrow label="Succeed"      visible={arrow3} />
        <StateBox label="SUCCEEDED" visible={succeededFade} active={step >= 4}   color={C.succeeded} />
      </div>

      <div style={{ marginTop: 56, opacity: fade(fps * 1.5, fps * 2.5), display: 'flex', gap: 36 }}>
        {[
          { label: 'SELECT FOR UPDATE SKIP LOCKED', sub: 'atomic claim' },
          { label: 'lease_token UUID', sub: 'ownership proof' },
          { label: 'heartbeat every 10s', sub: 'lease extension' },
        ].map(({ label, sub }) => (
          <div key={label} style={{ textAlign: 'center' }}>
            <div style={{ fontFamily: MONO, fontSize: 12, color: C.green, letterSpacing: '0.06em' }}>{label}</div>
            <div style={{ fontFamily: MONO, fontSize: 11, color: C.muted, marginTop: 4 }}>{sub}</div>
          </div>
        ))}
      </div>

    </AbsoluteFill>
  );
};
