// Scene 1 — Problem statement (0–5s)
// "What happens when a distributed worker crashes mid-job?"
import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, interpolate, Easing } from 'remotion';
import { C, SANS, MONO } from '../palette';

export const Scene01Problem: React.FC = () => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const fadeIn = interpolate(frame, [0, fps * 1.2], [0, 1], {
    extrapolateRight: 'clamp',
    easing: Easing.out(Easing.cubic),
  });
  const questionFade = interpolate(frame, [fps * 0.8, fps * 2.0], [0, 1], {
    extrapolateRight: 'clamp',
    easing: Easing.out(Easing.cubic),
  });
  const subFade = interpolate(frame, [fps * 1.8, fps * 3.2], [0, 1], {
    extrapolateRight: 'clamp',
    easing: Easing.out(Easing.cubic),
  });

  return (
    <AbsoluteFill style={{ background: C.bg, fontFamily: SANS, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>

      {/* Label */}
      <div style={{ opacity: fadeIn, transform: `translateY(${interpolate(fadeIn, [0, 1], [12, 0])}px)`, marginBottom: 28 }}>
        <span style={{ fontFamily: MONO, fontSize: 13, letterSpacing: '0.18em', color: C.muted, textTransform: 'uppercase' }}>
          FORGE — Distributed Job Execution
        </span>
      </div>

      {/* Main question */}
      <div style={{
        opacity: questionFade,
        transform: `translateY(${interpolate(questionFade, [0, 1], [20, 0])}px)`,
        textAlign: 'center',
        maxWidth: 860,
      }}>
        <h1 style={{ fontSize: 62, fontWeight: 700, color: C.text, lineHeight: 1.15, margin: 0, letterSpacing: '-0.02em' }}>
          What happens when a{' '}
          <span style={{ color: C.red }}>worker crashes</span>
          {' '}mid-job?
        </h1>
      </div>

      {/* Sub-question */}
      <div style={{
        opacity: subFade,
        transform: `translateY(${interpolate(subFade, [0, 1], [16, 0])}px)`,
        marginTop: 36,
        textAlign: 'center',
        maxWidth: 720,
      }}>
        <p style={{ fontSize: 26, color: C.muted, lineHeight: 1.6, margin: 0, fontWeight: 400 }}>
          PostgreSQL queue · lease-based ownership · embedded recovery scheduler
        </p>
      </div>

    </AbsoluteFill>
  );
};
