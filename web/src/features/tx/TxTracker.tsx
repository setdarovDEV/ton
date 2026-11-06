import { explorerTxUrl } from '@/lib/config';
import { shortHash } from '@/lib/format';
import type { Tx } from '@/lib/types';
import { Spinner } from '@/components/ui';
import type { FlowStep } from './useSendIntent';
import { useTxStatus } from './useTxStatus';

interface Props {
  step: FlowStep;
  tx: Tx | null;
  error: string | null;
}

/**
 * Visual stepper for the wallet → chain → confirmed lifecycle.
 * While a tx hash exists it polls the backend until the indexer confirms or fails it.
 */
export function TxTracker({ step, tx, error }: Props) {
  const live = useTxStatus(tx?.hash ?? null);
  const status = live.data?.status ?? tx?.status ?? null;

  const steps: Array<{ label: string; state: 'done' | 'active' | 'todo' | 'failed' }> = [
    { label: 'Sent to wallet', state: stageState(step, status, 0) },
    { label: 'Pending on chain', state: stageState(step, status, 1) },
    { label: status === 'failed' ? 'Failed' : 'Confirmed', state: stageState(step, status, 2) },
  ];

  return (
    <div className="rounded-2xl border border-zinc-800 bg-zinc-900/60 p-4">
      <ol className="flex items-center justify-between gap-2">
        {steps.map((s, i) => (
          <li key={s.label} className="flex flex-1 items-center gap-2">
            <StepDot state={s.state} index={i + 1} />
            <span className={`text-xs sm:text-sm ${s.state === 'todo' ? 'text-zinc-500' : 'text-zinc-100'}`}>{s.label}</span>
            {i < steps.length - 1 && <span className="mx-1 hidden h-px flex-1 bg-zinc-700 sm:block" aria-hidden="true" />}
          </li>
        ))}
      </ol>
      {tx && (
        <p className="mt-3 text-xs text-zinc-400">
          tx{' '}
          <a href={explorerTxUrl(tx.hash)} target="_blank" rel="noreferrer" className="font-mono text-sky-300 hover:underline">
            {shortHash(tx.hash)}
          </a>
          {status === 'pending' && ' — waiting for the indexer to confirm the on-chain owner change…'}
          {status === 'confirmed' && ' — confirmed.'}
          {status === 'failed' && ' — the transaction expired or did not produce the expected state change.'}
        </p>
      )}
      {step === 'error' && error && <p className="mt-3 text-xs text-rose-300">{error}</p>}
    </div>
  );
}

function stageState(step: FlowStep, status: Tx['status'] | null, idx: number): 'done' | 'active' | 'todo' | 'failed' {
  if (step === 'error') return idx === 0 ? 'failed' : 'todo';
  if (step === 'preparing' || step === 'wallet') return idx === 0 ? 'active' : 'todo';
  if (step === 'registering') return idx === 0 ? 'done' : idx === 1 ? 'active' : 'todo';
  // tracking
  if (status === 'confirmed') return 'done';
  if (status === 'failed') return idx === 2 ? 'failed' : 'done';
  return idx < 1 ? 'done' : idx === 1 ? 'active' : 'todo';
}

function StepDot({ state, index }: { state: 'done' | 'active' | 'todo' | 'failed'; index: number }) {
  const base = 'flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold';
  if (state === 'done') return <span className={`${base} bg-emerald-500/20 text-emerald-300`}>✓</span>;
  if (state === 'failed') return <span className={`${base} bg-rose-500/20 text-rose-300`}>✕</span>;
  if (state === 'active')
    return (
      <span className={`${base} bg-sky-500/20 text-sky-300`}>
        <Spinner className="h-4 w-4" />
      </span>
    );
  return <span className={`${base} bg-zinc-800 text-zinc-500`}>{index}</span>;
}
