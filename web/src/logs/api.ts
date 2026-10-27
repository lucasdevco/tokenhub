export type Metadata = {
  id?: string;
  provider?: string;
  user_agent?: string;
  method?: string;
  target_url?: string;
  source_url?: string;
  response_status_code?: number;
  model?: string;
  input_tokens?: number;
  output_tokens?: number;
  routing_overhead_ms?: number;
  time_to_first_token_ms?: number;
  tokens_per_second?: number;
  finish_reason?: string;
};

export type Transaction = {
  key: string;
  metadata: Metadata;
  started: string;
  state: string;
  capture_state?: string;
  capture_wait_ms?: number;
  request_file?: string;
  response_file?: string;
  request_bytes: number;
  response_bytes: number;
  duration_ms: number;
  metadata_files?: string[];
  problems?: string[];
  events?: unknown[];
};

export type CaptureFile = {
  name: string;
  size: number;
  modified: string;
  problem?: string;
};

export type RequestHistogram = {
  buckets: { started: string; count: number }[];
  interval_seconds: number;
  total: number;
};

export type RequestPage = {
  histogram: RequestHistogram;
  items: Transaction[];
  total: number;
  all: number;
  unfinished: number;
  errors: number;
  problem?: string;
  next_cursor?: string;
  indexing?: boolean;
};

export type FilePage = {
  items: CaptureFile[];
  total: number;
  problem?: string;
  next_cursor?: string;
  indexing?: boolean;
};

export type RawPage = {
  data: string;
  offset: number;
  next: number;
  size: number;
};

export async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal });
  if (!response.ok) throw Error(await response.text());
  return response.json();
}

export const downloadURL = (name: string) =>
  `/api/file?name=${encodeURIComponent(name)}&download=1`;
export const bytes = (value: number) =>
  value < 1024
    ? `${value} B`
    : value < 1048576
      ? `${(value / 1024).toFixed(1)} KiB`
      : `${(value / 1048576).toFixed(1)} MiB`;
export const timestamp = (value: string) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "—"
    : date.toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
};

const relativeFormatter = new Intl.RelativeTimeFormat("zh-CN", {
  numeric: "always",
});
export function relativeTime(value: string, now = Date.now()) {
  const seconds = (new Date(value).getTime() - now) / 1000;
  if (!Number.isFinite(seconds)) return "—";
  if (Math.abs(seconds) < 10) return "刚刚";
  for (const [unit, size] of [
    ["day", 86400],
    ["hour", 3600],
    ["minute", 60],
    ["second", 1],
  ] as const) {
    if (Math.abs(seconds) >= size)
      return relativeFormatter.format(Math.trunc(seconds / size), unit);
  }
  return "刚刚";
}
