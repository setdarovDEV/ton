import {
    Address,
    beginCell,
    Cell,
    Contract,
    contractAddress,
    ContractProvider,
    Dictionary,
    Sender,
    SendMode,
    toNano,
} from '@ton/core';
import { Codes } from './codes';

export const CollectionOps = {
    getRoyaltyParams: 0x693d3950,
    mint: 1,
    batchMint: 2,
    changeOwner: 3,
    changeContent: 4,
} as const;

export type RoyaltyParams = {
    /** royalty = price * factor / base */
    factor: number;
    base: number;
    address: Address;
};

export type NftCollectionConfig = {
    owner: Address;
    nextItemIndex: number;
    collectionContent: Cell;
    commonContent: Cell;
    nftItemCode: Cell;
    royalty: RoyaltyParams;
};

export type MintItem = {
    index: number;
    owner: Address;
    content: Cell;
    amount?: bigint;
};

export function nftCollectionConfigToCell(config: NftCollectionConfig): Cell {
    const content = beginCell().storeRef(config.collectionContent).storeRef(config.commonContent).endCell();
    const royalty = beginCell()
        .storeUint(config.royalty.factor, 16)
        .storeUint(config.royalty.base, 16)
        .storeAddress(config.royalty.address)
        .endCell();
    return beginCell()
        .storeAddress(config.owner)
        .storeUint(config.nextItemIndex, 64)
        .storeRef(content)
        .storeRef(config.nftItemCode)
        .storeRef(royalty)
        .endCell();
}

/** Item init payload expected by the collection on mint: owner + ref(individual content). */
function buildItemInitPayload(owner: Address, content: Cell): Cell {
    return beginCell().storeAddress(owner).storeRef(content).endCell();
}

export class NftCollection implements Contract {
    constructor(
        readonly address: Address,
        readonly init?: { code: Cell; data: Cell },
    ) {}

    static createFromAddress(address: Address): NftCollection {
        return new NftCollection(address);
    }

    static createFromConfig(config: NftCollectionConfig, code: Cell = Codes.nftCollection(), workchain = 0): NftCollection {
        const data = nftCollectionConfigToCell(config);
        const init = { code, data };
        return new NftCollection(contractAddress(workchain, init), init);
    }

    async sendDeploy(provider: ContractProvider, via: Sender, value: bigint): Promise<void> {
        await provider.internal(via, {
            value,
            sendMode: SendMode.PAY_GAS_SEPARATELY,
            body: beginCell().endCell(),
        });
    }

    async sendMint(
        provider: ContractProvider,
        via: Sender,
        opts: { value: bigint; queryId?: number; item: MintItem },
    ): Promise<void> {
        const itemAmount = opts.item.amount ?? toNano('0.05');
        await provider.internal(via, {
            value: opts.value,
            sendMode: SendMode.PAY_GAS_SEPARATELY,
            body: beginCell()
                .storeUint(CollectionOps.mint, 32)
                .storeUint(opts.queryId ?? 0, 64)
                .storeUint(opts.item.index, 64)
                .storeCoins(itemAmount)
                .storeRef(buildItemInitPayload(opts.item.owner, opts.item.content))
                .endCell(),
        });
    }

    async sendBatchMint(
        provider: ContractProvider,
        via: Sender,
        opts: { value: bigint; queryId?: number; items: MintItem[] },
    ): Promise<void> {
        if (opts.items.length === 0 || opts.items.length >= 250) {
            throw new Error('batch mint supports 1..249 items');
        }
        const dict = Dictionary.empty(Dictionary.Keys.Uint(64), Dictionary.Values.Cell());
        for (const item of opts.items) {
            dict.set(
                item.index,
                beginCell()
                    .storeCoins(item.amount ?? toNano('0.05'))
                    .storeRef(buildItemInitPayload(item.owner, item.content))
                    .endCell(),
            );
        }
        await provider.internal(via, {
            value: opts.value,
            sendMode: SendMode.PAY_GAS_SEPARATELY,
            body: beginCell()
                .storeUint(CollectionOps.batchMint, 32)
                .storeUint(opts.queryId ?? 0, 64)
                .storeDict(dict)
                .endCell(),
        });
    }

    async getCollectionData(provider: ContractProvider): Promise<{ nextItemIndex: number; content: Cell; owner: Address }> {
        const { stack } = await provider.get('get_collection_data', []);
        return {
            nextItemIndex: Number(stack.readBigNumber()),
            content: stack.readCell(),
            owner: stack.readAddress(),
        };
    }

    async getNftAddressByIndex(provider: ContractProvider, index: number): Promise<Address> {
        const { stack } = await provider.get('get_nft_address_by_index', [{ type: 'int', value: BigInt(index) }]);
        return stack.readAddress();
    }

    async getRoyaltyParams(provider: ContractProvider): Promise<RoyaltyParams> {
        const { stack } = await provider.get('royalty_params', []);
        return {
            factor: Number(stack.readBigNumber()),
            base: Number(stack.readBigNumber()),
            address: stack.readAddress(),
        };
    }
}
