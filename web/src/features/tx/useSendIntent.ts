import { useCallback, useState } from 'react';
import { useTonConnectUI } from '@tonconnect/ui-react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '@/api/endpoints';
import { keys } from '@/api/queryKeys';
import type { Intent, RegisterTxBody, Tx } from '@/lib/types';
import { useToast } from '@/components/Toast';

export type FlowStep = 'idle' | 'preparing' | 'wallet' | 'registering' | 'tracking' | 'error';

interface SendIntentState {
  step: FlowStep;
  tx: Tx | null;
  error: string | null;
}

/**
 * Orchestrates intent → wallet signature → tx registration.
 * Backend builds the messages (sale StateInit, transfer body); the frontend only forwards them to the wallet
 * and returns the signed external-message BOC, which the backend hashes and tracks.
 */
export function useSendIntent() {
  const [tonConnectUI] = useTonConnectUI();
  const qc = useQueryClient();
  const toast = useToast();
  const [state, setState] = useState<SendIntentState>({ step: 'idle', tx: null, error: null });

  const run = useCallback(
    async (
      prepare: () => Promise<Intent>,
      register: (intent: Intent, boc: string) => RegisterTxBody,
    ): Promise<Tx | null> => {
      setState({ step: 'preparing', tx: null, error: null });
      try {
        const intent = await prepare();
        setState((s) => ({ ...s, step: 'wallet' }));
        const result = await tonConnectUI.sendTransaction({
          validUntil: intent.valid_until,
          messages: intent.messages.map((m) => ({ address: m.address, amount: m.amount, payload: m.payload })),
        });
        setState((s) => ({ ...s, step: 'registering' }));
        const tx = await api.transactions.register(register(intent, result.boc));
        setState({ step: 'tracking', tx, error: null });
        void qc.invalidateQueries({ queryKey: keys.listingsRoot });
        void qc.invalidateQueries({ queryKey: keys.usersRoot });
        return tx;
      } catch (err) {
        const message = describeError(err);
        setState({ step: 'error', tx: null, error: message });
        toast.notify(message, 'error');
        return null;
      }
    },
    [tonConnectUI, qc, toast],
  );

  const reset = useCallback(() => setState({ step: 'idle', tx: null, error: null }), []);

  return { ...state, run, reset };
}

function describeError(err: unknown): string {
  if (err instanceof Error) {
    if (/reject|cancel|declin/i.test(err.message)) return 'Transaction was rejected in the wallet';
    return err.message;
  }
  return 'Transaction failed';
}
