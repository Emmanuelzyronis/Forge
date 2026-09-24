// Loads presentation.json at build time.
// Falls back to the fixture when FORGE_FIXTURE is set or presentation.json is absent.
import * as fs from 'fs';
import * as path from 'path';
import type { Presentation } from './types';

function loadData(): Presentation {
  const fixturePath = process.env.FORGE_FIXTURE
    ?? path.join(__dirname, '../../demo/fixtures/worker-crash-recovery.json');
  const livePath = path.join(__dirname, '../../demo/evidence/presentation.json');

  const candidates = [livePath, fixturePath];
  for (const p of candidates) {
    try {
      const raw = fs.readFileSync(p, 'utf8');
      return JSON.parse(raw) as Presentation;
    } catch {
      // try next
    }
  }
  throw new Error('No presentation.json found. Run `make demo-crash` first or set FORGE_FIXTURE.');
}

export const data: Presentation = loadData();

// Video timing: each second of real time = SECS_PER_FRAME * FPS frames
export const FPS = 60;
export const TOTAL_FRAMES = 5400; // 90 seconds at 60 fps

// Map a real elapsed_secs from the presentation into a video frame number.
// The first 3s of video is the Problem scene (fixed).
// Events are mapped into a 60-second window starting at frame 180.
const EVENT_OFFSET_FRAMES = FPS * 3; // 3s lead-in
const MAX_REAL_SECS = data.stats.total_duration_secs || 62;
const EVENT_WINDOW_FRAMES = FPS * 70; // 70 seconds for event timeline

export function elapsedToFrame(elapsedSecs: number): number {
  const frac = Math.min(elapsedSecs / MAX_REAL_SECS, 1);
  return Math.round(EVENT_OFFSET_FRAMES + frac * EVENT_WINDOW_FRAMES);
}
