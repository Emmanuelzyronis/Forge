// Presentation data is resolved at bundle time via the webpack alias
// 'presentation-data' configured in remotion.config.ts.
// The alias points to demo/evidence/presentation.json (live run) when it
// exists, or demo/fixtures/worker-crash-recovery.json (representative data)
// as a fallback. This keeps fs calls out of the browser bundle entirely.
//
// eslint-disable-next-line @typescript-eslint/no-require-imports
import type { Presentation } from './types';

// require() is safe here: webpack resolves 'presentation-data' to a static
// JSON file at bundle time, so no Node built-ins are needed at runtime.
// eslint-disable-next-line @typescript-eslint/no-var-requires, @typescript-eslint/no-require-imports
const jsonData = require('presentation-data');
export const data: Presentation = jsonData as Presentation;

export const FPS = 60;
export const TOTAL_FRAMES = 5400; // 90 seconds at 60 fps

// Map a real elapsed_secs from the presentation into a video frame number.
// The first 3s of video is the Problem scene (fixed).
// Events are mapped into a 60-second window starting at frame 180.
const EVENT_OFFSET_FRAMES = FPS * 3;
const MAX_REAL_SECS = data.stats.total_duration_secs || 62;
const EVENT_WINDOW_FRAMES = FPS * 70; // 70 seconds for event timeline

export function elapsedToFrame(elapsedSecs: number): number {
  const frac = Math.min(elapsedSecs / MAX_REAL_SECS, 1);
  return Math.round(EVENT_OFFSET_FRAMES + frac * EVENT_WINDOW_FRAMES);
}
