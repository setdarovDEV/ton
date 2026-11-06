import { Address, beginCell, Cell, Contract, contractAddress, ContractProvider, Sender, SendMode, toNano } from '@ton/core';
import { Codes } from './codes';

export const SaleOps = {
    acceptCoins: 1,
    buy: 2,
    cancel: 3,
    changePrice: 0x6c6c2080,
    emergency: 555,
} as const;

/** Minimum gas the sale contract demands on top of the price (min_gas_amount in the FunC source). */
export const SALE_MIN_GAS = toNano('0.1');

export type NftFixpriceSaleV3R3Config = {
    marketplace: Address;
    nft: Address;
    fullPrice: bigint;
    marketplaceFeeAddress: Address;
    marketplaceFee: bigint;
    royaltyAddress: Address | null;
    royaltyAmount: bigint;
    createdAt?: number;
    /** Set only when re-creating an existing sale's data cell; deploys always use addr_none. */
    nftOwner?: Address | null;
};

export type FixPriceData = {
    isComplete: boolean;
    createdAt: number;
    marketplace: Address;
    nft: Address;
    nftOwner: Address | null;
    fullPrice: bigint;
    marketplaceFeeAddress: Address;
    marketplaceFee: bigint;
    royaltyAddress: Address | null;
    royaltyAmount: bigint;
    soldAt: number;
    queryId: bigint;
};

export function nftFixpriceSaleV3R3ConfigToCell(config: NftFixpriceSaleV3R3Config): Cell {
    const fees = beginCell()
        .storeAddress(config.marketplaceFeeAddress)
        .storeCoins(config.marketplaceFee)
        .storeAddress(config.royaltyAddress)
        .storeCoins(config.royaltyAmount)
        .endCell();
    return beginCell()
        .storeUint(0, 1) // is_complete
        .storeUint(config.createdAt ?? Math.floor(Date.now() / 1000), 32)
        .storeAddress(config.marketplace)
        .storeAddress(config.nft)
        .storeAddress(config.nftOwner ?? null) // addr_none until ownership_assigned arrives
        .storeCoins(config.fullPrice)
        .storeRef(fees)
        .storeUint(0, 32) // sold_at
        .storeUint(0, 64) // query_id
        .endCell();
}

/** Body for a buy with an explicit query_id (op 2). An empty body (op 0) also buys. */
export function buildBuyBody(queryId: number | bigint = 0): Cell {
    return beginCell().storeUint(SaleOps.buy, 32).storeUint(queryId, 64).endCell();
}

/** Body that the marketplace forwards to the sale on deploy ("just accept coins"). */
export function buildAcceptCoinsBody(queryId: number | bigint = 0): Cell {
    return beginCell().storeUint(SaleOps.acceptCoins, 32).storeUint(queryId, 64).endCell();
}

export function buildCancelBody(queryId: number | bigint = 0): Cell {
    return beginCell().storeUint(SaleOps.cancel, 32).storeUint(queryId, 64).endCell();
}

export class NftFixpriceSaleV3R3 implements Contract {
    constructor(
        readonly address: Address,
        readonly init?: { code: Cell; data: Cell },
    ) {}

    static createFromAddress(address: Address): NftFixpriceSaleV3R3 {
        return new NftFixpriceSaleV3R3(address);
    }

    static createFromConfig(
        config: NftFixpriceSaleV3R3Config,
        code: Cell = Codes.fixpriceSaleV3R3(),
        workchain = 0,
    ): NftFixpriceSaleV3R3 {
        const data = nftFixpriceSaleV3R3ConfigToCell(config);
        const init = { code, data };
        return new NftFixpriceSaleV3R3(contractAddress(workchain, init), init);
    }

    /** StateInit cell as embedded into the marketplace deploy message. */
    stateInitCell(): Cell {
        if (!this.init) {
            throw new Error('sale has no init');
        }
        return beginCell()
            .storeUint(0, 2) // no split_depth, no special
            .storeMaybeRef(this.init.code)
            .storeMaybeRef(this.init.data)
            .storeUint(0, 1) // no libraries
            .endCell();
    }

    async sendBuy(
        provider: ContractProvider,
        via: Sender,
        opts: { price: bigint; extraGas?: bigint; queryId?: number | bigint },
    ): Promise<void> {
        await provider.internal(via, {
            value: opts.price + (opts.extraGas ?? toNano('0.15')),
            sendMode: SendMode.PAY_GAS_SEPARATELY,
            body: buildBuyBody(opts.queryId ?? 0),
        });
    }

    async sendCancel(provider: ContractProvider, via: Sender, opts: { queryId?: number | bigint } = {}): Promise<void> {
        await provider.internal(via, {
            value: SALE_MIN_GAS,
            sendMode: SendMode.PAY_GAS_SEPARATELY,
            body: buildCancelBody(opts.queryId ?? 0),
        });
    }

    async getFixPriceData(provider: ContractProvider): Promise<FixPriceData> {
        const { stack } = await provider.get('get_fix_price_data', []);
        return {
            isComplete: stack.readBigNumber() !== 0n,
            createdAt: Number(stack.readBigNumber()),
            marketplace: stack.readAddress(),
            nft: stack.readAddress(),
            nftOwner: stack.readAddressOpt(),
            fullPrice: stack.readBigNumber(),
            marketplaceFeeAddress: stack.readAddress(),
            marketplaceFee: stack.readBigNumber(),
            royaltyAddress: stack.readAddressOpt(),
            royaltyAmount: stack.readBigNumber(),
            soldAt: Number(stack.readBigNumber()),
            queryId: stack.readBigNumber(),
        };
    }
}
