// Scene 3 — Worker-01 crashes (frames 780–1500 = 13–25s)
import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, interpolate, Easing, spring } from 'remotion';
import { C, SANS, MONO } from '../palette';

export const Scene03WorkerFailure: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const titleFade = interpolate(frame, [0, fps * 0.8], [0, 1], { extrapolateRight: 'clamp' });
  const workerFade = interpolate(frame, [fps * 0.4, fps * 1.2], [0, 1], { extrapolateRight: 'clamp' });

  // Crash flash at 3.5s in scene
  const crashAt = fps * 3.5;
  const crashFlash = interpolate(frame, [crashAt, crashAt + 8, crashAt + 20], [0, 1, 0], { extrapolateRight: 'clamp' });
  const crashLabelFade = interpolate(frame, [crashAt + 5, crashAt + fps * 0.6], [0, 1], { extrapolateRight: 'clamp' });

  const shake = frame > crashAt && frame < crashAt + fps * 0.3
    ? Math.sin((frame - crashAt) * 1.8) * interpolate(frame, [crashAt, crashAt + fps * 0.3], [8, 0], { extrapolateRight: 'clamp' })
    : 0;

  const postCrashFade = interpolate(frame, [crashAt + fps * 0.5, crashAt + fps * 1.2], [0, 1], { extrapolateRight: 'clamp' });

  const workerDimmed = frame > crashAt ? interpolate(frame, [crashAt, crashAt + fps * 0.6], [1, 0.25], { extrapolateRight: 'clamp' }) : 1;

  return (
    <AbsoluteFill style={{
      background: C.bg,
      fontFamily: SANS,
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      overflow: 'hidden',
    }}>

      {/* Crash flash overlay */}
      <div style={{
        position: 'absolute', inset: 0,
        background: C.red,
        opacity: crashFlash * 0.15,
        pointerEvents: 'none',
      }} />

      <div style={{ opacity: titleFade, marginBottom: 48, textAlign: 'center' }}>
        <span style={{ fontFamily: MONO, fontSize: 12, letterSpacing: '0.2em', color: C.muted, textTransform: 'uppercase' }}>
          Failure Scenario — Worker Crash
        </span>
      </div>

      {/* Worker-01 card */}
      <div style={{
        opacity: workerFade,
        transform: `translateX(${shake}px)`,
        background: C.surface,
        border: `1.5px solid ${frame > crashAt ? C.red : C.border}`,
        borderRadius: 8,
        padding: '28px 48px',
        minWidth: 340,
        textAlign: 'center',
        transition: 'border-color 0.2s',
        opacity: workerDimmed,
      }}>
        <div style={{ fontFamily: MONO, fontSize: 13, color: C.muted, letterSpacing: '0.12em', marginBottom: 10 }}>WORKER</div>
        <div style={{ fontFamily: MONO, fontSize: 28, fontWeight: 700, color: frame > crashAt ? C.red : C.text }}>
          worker-01
        </div>
        <div style={{ fontFamily: MONO, fontSize: 12, color: frame > crashAt ? C.red : C.running, marginTop: 8 }}>
          {frame > crashAt ? 'OFFLINE — process killed' : 'RUNNING — executing job'}
        </div>
        <div style={{ fontFamily: MONO, fontSize: 11, color: C.muted, marginTop: 4 }}>
          {frame > crashAt ? 'no sentinel write — crash is silent' : 'heartbeat every 8s · lease_expires_at extending'}
        </div>
      </div>

      {/* Crash label */}
      {frame > crashAt && (
        <div style={{ opacity: crashLabelFade, marginTop: 32, textAlign: 'center' }}>
          <div style={{ fontFamily: MONO, fontSize: 22, fontWeight: 700, color: C.red, letterSpacing: '0.08em' }}>
            SIGKILL — process terminated
          </div>
          <div style={{ fontFamily: MONO, fontSize: 14, color: C.muted, marginTop: 10 }}>
            heartbeats stop · lease_expires_at will not be renewed
          </div>
        </div>
      )}

      {/* Explanation */}
      <div style={{ opacity: postCrashFade, marginTop: 40, maxWidth: 620, textAlign: 'center' }}>
        <p style={{ fontSize: 18, color: C.muted, lineHeight: 1.7, margin: 0 }}>
          No crash sentinel is required. The lease expires by inaction.
          The recovery scheduler detects it on its next sweep.
        </p>
      </div>

    </AbsoluteFill>
  );
};
