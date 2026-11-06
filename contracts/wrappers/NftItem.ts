import { Address, beginCell, Cell, Contract, ContractProvider, Sender, SendMode } from '@ton/core';

export const NftItemOps = {
    transfer: 0x5fcc3d14,
    ownershipAssigned: 0x05138d91,
    excesses: 0xd53276db,
    getStaticData: 0x2fcb26a2,
} as const;

export type NftTransfer = {
    newOwner: Address;
    responseDestination: Address | null;
    forwardAmount: bigint;
    forwardPayload?: Cell;
    queryId?: number | bigint;
};

/** TEP-62 transfer body. */
export function buildNftTransferBody(t: NftTransfer): Cell {
    const builder = beginCell()
        .storeUint(NftItemOps.transfer, 32)
        .storeUint(t.queryId ?? 0, 64)
        .storeAddress(t.newOwner)
        .storeAddress(t.responseDestination)
        .storeBit(false) // no custom_payload
        .storeCoins(t.forwardAmount);
    if (t.forwardPayload) {
        builder.storeBit(true).storeRef(t.forwardPayload);
    } else {
        builder.storeBit(false);
    }
    return builder.endCell();
}

export class NftItem implements Contract {
    constructor(readonly address: Address) {}

    static createFromAddress(address: Address): NftItem {
        return new NftItem(address);
    }

    async sendTransfer(provider: ContractProvider, via: Sender, opts: { value: bigint } & NftTransfer): Promise<void> {
        await provider.internal(via, {
            value: opts.value,
            sendMode: SendMode.PAY_GAS_SEPARATELY,
            body: buildNftTransferBody(opts),
        });
    }

    async getNftData(provider: ContractProvider): Promise<{
        initialized: boolean;
        index: number;
        collection: Address | null;
        owner: Address | null;
        content: Cell | null;
    }> {
        const { stack } = await provider.get('get_nft_data', []);
        const initialized = stack.readBigNumber() !== 0n;
        const index = Number(stack.readBigNumber());
        const collection = stack.readAddressOpt();
        const owner = stack.readAddressOpt();
        const content = stack.readCellOpt();
        return { initialized, index, collection, owner, content };
    }
}
