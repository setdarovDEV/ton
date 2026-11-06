import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTonConnectUI, useTonWallet, type Wallet } from '@tonconnect/ui-react';
import { api } from '@/api/endpoints';
import { setTokenProvider } from '@/api/client';
import { storageGet, storageRemove, storageSet } from '@/lib/storage';
import { expectedChainId } from '@/lib/config';
import { useToast } from '@/components/Toast';
import { decodeJwtSubject, isJwtExpired } from './jwt';

const TOKEN_KEY = 'tonnet.jwt';
/** TON Connect recommends refreshing the proof payload roughly every 10 minutes. */
const PAYLOAD_TTL_MS = 10 * 60 * 1000;

interface AuthState {
  /** Raw address ("0:hex") of the connected wallet, or null. */
  rawAddress: string | null;
  /** Wallet is connected AND the backend has verified ton_proof (JWT present). */
  isVerified: boolean;
  wallet: Wallet | null;
  token: string | null;
  disconnect: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

/**
 * Drives the TON Connect `ton_proof` handshake:
 *  1. before connect: ask backend for a payload, pass it as connect request parameter;
 *  2. after connect: send proof + state_init to backend, receive JWT;
 *  3. keep the JWT in localStorage (best effort) and expose it to the API client.
 */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [tonConnectUI] = useTonConnectUI();
  const wallet = useTonWallet();
  const toast = useToast();
  const [token, setToken] = useState<string | null>(() => storageGet(TOKEN_KEY));
  const verifying = useRef(false);

  const rawAddress = wallet?.account.address ?? null;

  // Token is only valid for the currently connected wallet.
  const effectiveToken = useMemo(() => {
    if (!token || !rawAddress) return null;
    if (isJwtExpired(token)) return null;
    const sub = decodeJwtSubject(token);
    return sub && sub.toLowerCase() === rawAddress.toLowerCase() ? token : null;
  }, [token, rawAddress]);

  useEffect(() => {
    setTokenProvider(() => effectiveToken);
  }, [effectiveToken]);

  const persistToken = useCallback((t: string | null) => {
    setToken(t);
    if (t) storageSet(TOKEN_KEY, t);
    else storageRemove(TOKEN_KEY);
  }, []);

  // Step 1: prepare the ton_proof payload while the wallet is disconnected; refresh periodically.
  useEffect(() => {
    if (wallet) return;
    let cancelled = false;

    const refresh = async () => {
      tonConnectUI.setConnectRequestParameters({ state: 'loading' });
      try {
        const { payload } = await api.auth.payload();
        if (!cancelled) tonConnectUI.setConnectRequestParameters({ state: 'ready', value: { tonProof: payload } });
      } catch {
        // Backend unreachable: allow plain connect so browsing still works; writes will require re-connect.
        if (!cancelled) tonConnectUI.setConnectRequestParameters(null);
      }
    };

    void refresh();
    const timer = window.setInterval(() => void refresh(), PAYLOAD_TTL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [tonConnectUI, wallet]);

  // Step 2: exchange the proof for a JWT once the wallet connects.
  useEffect(() => {
    if (!wallet) return;
    if (effectiveToken || verifying.current) return;

    const proofItem = wallet.connectItems?.tonProof;
    if (!proofItem || !('proof' in proofItem)) {
      // Wallet returned no proof (e.g. restored session). Keep browsing; user must reconnect to sign.
      return;
    }
    if (wallet.account.chain !== expectedChainId) {
      toast.notify(`Wrong network: switch your wallet to ${expectedChainId === '-3' ? 'testnet' : 'mainnet'}`, 'error');
      return;
    }

    verifying.current = true;
    api.auth
      .verify({
        address: wallet.account.address,
        network: wallet.account.chain,
        public_key: wallet.account.publicKey ?? '',
        proof: proofItem.proof,
        state_init: wallet.account.walletStateInit,
      })
      .then((res) => {
        persistToken(res.token);
        toast.notify('Wallet verified', 'success');
      })
      .catch((err) => {
        toast.error(err, 'Wallet verification failed');
        void tonConnectUI.disconnect();
      })
      .finally(() => {
        verifying.current = false;
      });
  }, [wallet, effectiveToken, persistToken, toast, tonConnectUI]);

  // Drop the session token as soon as the wallet disconnects.
  useEffect(() => {
    return tonConnectUI.onStatusChange((w) => {
      if (!w) persistToken(null);
    });
  }, [tonConnectUI, persistToken]);

  const disconnect = useCallback(async () => {
    persistToken(null);
    await tonConnectUI.disconnect();
  }, [persistToken, tonConnectUI]);

  const value = useMemo<AuthState>(
    () => ({ rawAddress, isVerified: !!effectiveToken, wallet, token: effectiveToken, disconnect }),
    [rawAddress, effectiveToken, wallet, disconnect],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
