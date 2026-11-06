import { Address, beginCell, Cell, Contract, contractAddress, ContractProvider, Sender, SendMode } from '@ton/core';
import { sign } from '@ton/crypto';
import { Codes } from './codes';

export const MarketplaceOps = {
    deploySale: 1,
} as const;

export type NftMarketplaceV2Config = {
    seqno: number;
    subwallet: number;
    publicKey: Buffer;
};

export function nftMarketplaceV2ConfigToCell(config: NftMarketplaceV2Config): Cell {
    if (config.publicKey.length !== 32) {
        throw new Error('public key must be 32 bytes');
    }
    return beginCell().storeUint(config.seqno, 32).storeUint(config.subwallet, 32).storeBuffer(config.publicKey).endCell();
}

/** The cell whose hash the marketplace verifies: ref(stateInit) ref(body) with no data bits. */
export function deployPayloadCell(stateInit: Cell, body: Cell): Cell {
    return beginCell().storeRef(stateInit).storeRef(body).endCell();
}

/**
 * Builds the op=1 message body: op, 512-bit Ed25519 signature over hash(ref stateInit, ref body),
 * then the two refs. Mirrors `recv_internal` in nft-marketplace-v2.fc.
 */
export function buildSignedDeployBody(stateInit: Cell, body: Cell, secretKey: Buffer): Cell {
    const signature = sign(deployPayloadCell(stateInit, body).hash(), secretKey);
    return beginCell()
        .storeUint(MarketplaceOps.deploySale, 32)
        .storeBuffer(signature)
        .storeRef(stateInit)
        .storeRef(body)
        .endCell();
}

export class NftMarketplaceV2 implements Contract {
    constructor(
        readonly address: Address,
        readonly init?: { code: Cell; data: Cell },
    ) {}

    static createFromAddress(address: Address): NftMarketplaceV2 {
        return new NftMarketplaceV2(address);
    }

    static createFromConfig(config: NftMarketplaceV2Config, code: Cell = Codes.marketplaceV2(), workchain = 0): NftMarketplaceV2 {
        const data = nftMarketplaceV2ConfigToCell(config);
        const init = { code, data };
        return new NftMarketplaceV2(contractAddress(workchain, init), init);
    }

    async sendDeploy(provider: ContractProvider, via: Sender, value: bigint): Promise<void> {
        await provider.internal(via, {
            value,
            sendMode: SendMode.PAY_GAS_SEPARATELY,
            body: beginCell().endCell(), // empty body is ignored by the contract
        });
    }

    async sendDeploySale(
        provider: ContractProvider,
        via: Sender,
        opts: { value: bigint; stateInit: Cell; body: Cell; secretKey: Buffer },
    ): Promise<void> {
        await provider.internal(via, {
            value: opts.value,
            sendMode: SendMode.PAY_GAS_SEPARATELY,
            body: buildSignedDeployBody(opts.stateInit, opts.body, opts.secretKey),
        });
    }

    async getSeqno(provider: ContractProvider): Promise<number> {
        const { stack } = await provider.get('seqno', []);
        return Number(stack.readBigNumber());
    }

    async getPublicKey(provider: ContractProvider): Promise<Buffer> {
        const { stack } = await provider.get('get_public_key', []);
        return Buffer.from(stack.readBigNumber().toString(16).padStart(64, '0'), 'hex');
    }
}
