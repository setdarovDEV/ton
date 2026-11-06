const NANO = 1_000_000_000n;

/** Formats a nanoTON amount (string or bigint) as a TON decimal, trimming trailing zeros. */
export function formatTon(nano: string | bigint | null | undefined, maxFraction = 4): string {
  if (nano === null || nano === undefined || nano === '') return '—';
  let value: bigint;
  try {
    value = typeof nano === 'bigint' ? nano : BigInt(nano);
  } catch {
    return '—';
  }
  const negative = value < 0n;
  if (negative) value = -value;
  const whole = value / NANO;
  const frac = (value % NANO).toString().padStart(9, '0').slice(0, maxFraction).replace(/0+$/, '');
  return `${negative ? '-' : ''}${whole.toLocaleString('en-US')}${frac ? '.' + frac : ''}`;
}

/** Parses a human TON amount ("1.5") into nanoTON. Throws on invalid input. */
export function parseTon(input: string): bigint {
  const trimmed = input.trim();
  if (!/^\d+(\.\d{0,9})?$/.test(trimmed)) {
    throw new Error('Enter a positive TON amount with at most 9 decimals');
  }
  const [whole, frac = ''] = trimmed.split('.');
  return BigInt(whole) * NANO + BigInt(frac.padEnd(9, '0'));
}

export function shortAddr(address: string, head = 6, tail = 4): string {
  if (!address) return '';
  return address.length <= head + tail + 1 ? address : `${address.slice(0, head)}…${address.slice(-tail)}`;
}

export function shortHash(hash: string): string {
  return shortAddr(hash, 8, 6);
}

export function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString();
}

export function sameRaw(a?: string | null, b?: string | null): boolean {
  return !!a && !!b && a.toLowerCase() === b.toLowerCase();
}
