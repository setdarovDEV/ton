import { config } from '@/lib/config';

/** Error envelope returned by the Go API: { error: { code, message } }. */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

type TokenProvider = () => string | null;

let tokenProvider: TokenProvider = () => null;

/** Lets the auth layer inject the bearer token without coupling the client to React state. */
export function setTokenProvider(provider: TokenProvider): void {
  tokenProvider = provider;
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'DELETE';
  body?: unknown;
  auth?: boolean;
  signal?: AbortSignal;
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
  if (opts.auth) {
    const token = tokenProvider();
    if (!token) throw new ApiError(401, 'unauthenticated', 'Connect your wallet and complete sign-in first');
    headers.Authorization = `Bearer ${token}`;
  }

  const res = await fetch(`${config.apiUrl}${path}`, {
    method: opts.method ?? 'GET',
    headers,
    body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    signal: opts.signal,
  });

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  let json: unknown = null;
  if (text) {
    try {
      json = JSON.parse(text);
    } catch {
      json = null;
    }
  }

  if (!res.ok) {
    const env = json as { error?: { code?: string; message?: string } } | null;
    throw new ApiError(res.status, env?.error?.code ?? `http_${res.status}`, env?.error?.message ?? res.statusText);
  }
  return json as T;
}

export function qs(params: Record<string, string | number | undefined | null>): string {
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : '';
}
