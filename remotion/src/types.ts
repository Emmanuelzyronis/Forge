// Types consumed by every scene — mirrors demo/collect/main.go Presentation struct

export interface TimelineEntry {
  elapsed_secs: number;
  event_type: string;
  from_state?: string;
  to_state?: string;
  worker_name?: string;
  attempt_num?: number;
  is_crash?: boolean;
  is_recovery?: boolean;
}

export interface PresentationAttempt {
  attempt_num: number;
  worker_name: string;
  state: string;
  duration_ms?: number;
  fail_reason?: string;
}

export interface Worker {
  ID: string;
  Name: string;
  State: string;
  LastSeen?: string;
}

export interface PresentationStats {
  total_duration_secs: number;
  recovery_latency_secs: number;
  lease_duration_secs: number;
  recovery_interval_secs: number;
  attempt_count: number;
  max_attempts: number;
}

export interface APIJob {
  ID: string;
  Kind: string;
  State: string;
  AttemptCount: number;
  MaxAttempts: number;
  CreatedAt: string;
  UpdatedAt: string;
  TerminalAt?: string;
}

export interface Presentation {
  scenario: string;
  generated_at: string;
  run_id: string;
  job: APIJob;
  timeline: TimelineEntry[];
  attempts: PresentationAttempt[];
  workers: Worker[];
  stats: PresentationStats;
}
