/** Minimal, non-verifying JWT claim reader. Verification happens on the backend. */
function decodeClaims(token: string): Record<string, unknown> | null {
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  try {
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    const json = atob(b64.padEnd(b64.length + ((4 - (b64.length % 4)) % 4), '='));
    return JSON.parse(json) as Record<string, unknown>;
  } catch {
    return null;
  }
}

export function decodeJwtSubject(token: string): string | null {
  const sub = decodeClaims(token)?.sub;
  return typeof sub === 'string' ? sub : null;
}

export function isJwtExpired(token: string, skewSeconds = 30): boolean {
  const exp = decodeClaims(token)?.exp;
  if (typeof exp !== 'number') return false;
  return exp * 1000 < Date.now() + skewSeconds * 1000;
}
