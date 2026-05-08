export interface BootConfig {
  version: string
  csrf: string
  user: string
  port: number
}

declare global {
  interface Window {
    __TUNERR_BOOT__?: BootConfig
  }
}

export function boot(): BootConfig {
  return window.__TUNERR_BOOT__ ?? { version: 'dev', csrf: '', user: '', port: 48879 }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const csrf = boot().csrf
  if (csrf) headers['X-IPTVTunerr-CSRF'] = csrf

  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(`${method} ${path}: ${res.status} ${text}`)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}
