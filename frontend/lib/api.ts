// Typed client for the Go API (net/http). All calls go to /api/*, which
// next.config.mjs proxies to the Go server.

export type Verdict = "halts" | "loops" | "unknown" | "crash";

export interface RunResult {
  verdict: Verdict;
  steps: number;
  limit: number;
  limitAdjusted: boolean;
  final?: string;
  message?: string;
  loopLine?: number;
  firstSeen?: number;
  repeatedAt?: number;
  state?: string;
  trail: string[];
  elided: number; // entries dropped between the first 12 and last 3 of trail
  notes?: string[];
}

export interface Preset {
  id: string;
  name: string;
  sub: string;
  desc: string;
  code: string;
  start: number;
  limit: number;
}

export class ApiError extends Error {
  line?: number;
  constructor(message: string, line?: number) {
    super(message);
    this.line = line;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), 10_000);
  let res: Response;
  try {
    res = await fetch(path, { ...init, signal: ctrl.signal });
  } catch {
    throw new ApiError("Can’t reach the server. Make sure the Go backend is running (see the README).");
  } finally {
    clearTimeout(timer);
  }
  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    /* non-JSON body, handled below */
  }
  if (!res.ok) {
    const e = (body as { error?: { message?: string; line?: number } } | null)?.error;
    throw new ApiError(e?.message ?? `The server returned an error (${res.status}).`, e?.line);
  }
  return body as T;
}

function post<T>(path: string, payload: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export const getPresets = () => request<Preset[]>("/api/programs");
export const simulate = (p: { program: string; start: number; limit: number }) => post<RunResult>("/api/simulate", p);
export const checkCode = (p: { code: string; limit: number }) => post<RunResult>("/api/check", p);
