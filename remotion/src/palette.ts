// Design tokens — dark engineering palette
export const C = {
  bg:       '#0A0C0F',
  surface:  '#111417',
  border:   '#1E2328',
  divider:  '#252A30',

  text:     '#E8EAF0',
  muted:    '#6C7380',
  faint:    '#3A3F47',

  blue:     '#3B82F6',
  green:    '#22C55E',
  yellow:   '#EAB308',
  orange:   '#F97316',
  red:      '#EF4444',
  teal:     '#14B8A6',

  // State colours
  queued:   '#3B82F6',
  claimed:  '#EAB308',
  running:  '#22C55E',
  succeeded:'#10B981',
  failed:   '#EF4444',
  recovery: '#F97316',
  crash:    '#EF4444',
} as const;

export function stateColor(state: string): string {
  const map: Record<string, string> = {
    QUEUED:    C.queued,
    CLAIMED:   C.claimed,
    RUNNING:   C.running,
    SUCCEEDED: C.succeeded,
    FAILED:    C.failed,
    CRASH:     C.crash,
    ABANDONED: C.orange,
    RECOVERY:  C.recovery,
  };
  return map[state] ?? C.muted;
}

export const MONO = "'JetBrains Mono', 'Fira Code', 'Menlo', monospace";
export const SANS = "'Inter', 'SF Pro Display', 'Helvetica Neue', sans-serif";
