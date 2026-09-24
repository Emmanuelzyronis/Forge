// Scene 5 — Recovery: re-queue + worker-02 picks up (frames 2220–3300 = 37–55s)
import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, interpolate, Easing } from 'remotion';
import { C, SANS, MONO, stateColor } from '../palette';
import { data } from '../data';

export const Scene05Recovery: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const fade = (start: number, end: number) =>
    interpolate(frame, [start * fps, end * fps], [0, 1], { extrapolateRight: 'clamp', easing: Easing.out(Easing.cubic) });

  const titleFade      = fade(0, 0.6);
  const abandonedFade  = fade(0.4, 1.0);
  const requeueFade    = fade(1.2, 1.8);
  const worker2Fade    = fade(2.0, 2.6);
  const claimedFade    = fade(2.8, 3.4);
  const runningFade    = fade(4.0, 4.6);

  const recoveryLatency = data.stats.recovery_latency_secs;

  return (
    <AbsoluteFill style={{ background: C.bg, fontFamily: SANS, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 0 }}>

      <div style={{ opacity: titleFade, marginBottom: 44, textAlign: 'center' }}>
        <span style={{ fontFamily: MONO, fontSize: 12, letterSpacing: '0.2em', color: C.muted, textTransform: 'uppercase' }}>
          Recovery — Attempt 2
        </span>
      </div>

      {/* Recovery timeline */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 20, width: 640 }}>

        {/* ABANDONED event */}
        <div style={{ opacity: abandonedFade, display: 'flex', alignItems: 'center', gap: 18 }}>
          <div style={{ width: 10, height: 10, borderRadius: '50%', background: C.orange, flexShrink: 0 }} />
          <div>
            <span style={{ fontFamily: MONO, fontSize: 15, fontWeight: 700, color: C.orange }}>ABANDONED</span>
            <span style={{ fontFamily: MONO, fontSize: 12, color: C.muted, marginLeft: 16 }}>
              attempt 1 · lease expired after {recoveryLatency}s
            </span>
          </div>
        </div>

        {/* RE-QUEUE */}
        <div style={{ opacity: requeueFade, display: 'flex', alignItems: 'center', gap: 18 }}>
          <div style={{ width: 10, height: 10, borderRadius: '50%', background: C.blue, flexShrink: 0 }} />
          <div>
            <span style={{ fontFamily: MONO, fontSize: 15, fontWeight: 700, color: C.blue }}>REQUEUED</span>
            <span style={{ fontFamily: MONO, fontSize: 12, color: C.muted, marginLeft: 16 }}>
              eligible_at = NOW() + 5s · attempt_count = 1 &lt; max_attempts = 3
            </span>
          </div>
        </div>

        {/* Worker-02 starts */}
        <div style={{ opacity: worker2Fade, display: 'flex', alignItems: 'center', gap: 18 }}>
          <div style={{ width: 10, height: 10, borderRadius: '50%', background: C.green, flexShrink: 0 }} />
          <div>
            <span style={{ fontFamily: MONO, fontSize: 15, fontWeight: 700, color: C.green }}>worker-02 ONLINE</span>
            <span style={{ fontFamily: MONO, fontSize: 12, color: C.muted, marginLeft: 16 }}>
              registered · polling for eligible jobs
            </span>
          </div>
        </div>

        {/* CLAIMED by worker-02 */}
        <div style={{ opacity: claimedFade, display: 'flex', alignItems: 'center', gap: 18 }}>
          <div style={{ width: 10, height: 10, borderRadius: '50%', background: C.claimed, flexShrink: 0 }} />
          <div>
            <span style={{ fontFamily: MONO, fontSize: 15, fontWeight: 700, color: C.claimed }}>CLAIMED by worker-02</span>
            <span style={{ fontFamily: MONO, fontSize: 12, color: C.muted, marginLeft: 16 }}>
              new lease_token · stale worker-01 token invalidated
            </span>
          </div>
        </div>

        {/* RUNNING */}
        <div style={{ opacity: runningFade, display: 'flex', alignItems: 'center', gap: 18 }}>
          <div style={{ width: 10, height: 10, borderRadius: '50%', background: C.running, flexShrink: 0 }} />
          <div>
            <span style={{ fontFamily: MONO, fontSize: 15, fontWeight: 700, color: C.running }}>RUNNING</span>
            <span style={{ fontFamily: MONO, fontSize: 12, color: C.muted, marginLeft: 16 }}>
              attempt 2 · heartbeat active
            </span>
          </div>
        </div>

      </div>

      {/* Key invariant */}
      <div style={{
        opacity: fade(5.5, 6.5),
        marginTop: 40,
        background: `${C.green}0A`,
        border: `1px solid ${C.green}22`,
        borderRadius: 6,
        padding: '16px 28px',
        maxWidth: 620,
        textAlign: 'center',
      }}>
        <div style={{ fontFamily: MONO, fontSize: 11, color: C.green, letterSpacing: '0.12em', marginBottom: 8 }}>F-INV-006</div>
        <div style={{ fontFamily: MONO, fontSize: 13, color: C.muted, lineHeight: 1.6 }}>
          If worker-01 comes back and tries to succeed its old attempt,
          the lease token mismatch rejects it with 409 Conflict.
        </div>
      </div>

    </AbsoluteFill>
  );
};
