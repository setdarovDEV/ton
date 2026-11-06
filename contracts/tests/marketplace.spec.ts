import { Blockchain, SandboxContract, TreasuryContract } from '@ton/sandbox';
import { Address, beginCell, Cell, toNano } from '@ton/core';
import { KeyPair, keyPairFromSeed, getSecureRandomBytes } from '@ton/crypto';
import '@ton/test-utils';

import {
    buildAcceptCoinsBody,
    buildCommonContent,
    buildItemContent,
    buildOffchainContent,
    buildSignedDeployBody,
    Codes,
    NftCollection,
    NftFixpriceSaleV3R3,
    NftItem,
    NftMarketplaceV2,
    SALE_MIN_GAS,
} from '../wrappers';

const PRICE = toNano('5');
const MARKETPLACE_FEE = toNano('0.25'); // 5%
const ROYALTY = toNano('0.25'); // 5%
const DEPLOY_VALUE = toNano('0.05');
const TRANSFER_VALUE = toNano('0.1');
const FORWARD_AMOUNT = toNano('0.05');

type Fixture = {
    blockchain: Blockchain;
    deployer: SandboxContract<TreasuryContract>;
    seller: SandboxContract<TreasuryContract>;
    buyer: SandboxContract<TreasuryContract>;
    feeWallet: SandboxContract<TreasuryContract>;
    royaltyWallet: SandboxContract<TreasuryContract>;
    marketplaceKeys: KeyPair;
    marketplace: SandboxContract<NftMarketplaceV2>;
    collection: SandboxContract<NftCollection>;
    nft: SandboxContract<NftItem>;
};

async function setupFixture(): Promise<Fixture> {
    const blockchain = await Blockchain.create();
    const deployer = await blockchain.treasury('deployer');
    const seller = await blockchain.treasury('seller');
    const buyer = await blockchain.treasury('buyer');
    const feeWallet = await blockchain.treasury('fee');
    const royaltyWallet = await blockchain.treasury('royalty');

    const marketplaceKeys = keyPairFromSeed(await getSecureRandomBytes(32));
    const marketplace = blockchain.openContract(
        NftMarketplaceV2.createFromConfig({ seqno: 0, subwallet: 0, publicKey: marketplaceKeys.publicKey }),
    );
    const mpDeploy = await marketplace.sendDeploy(deployer.getSender(), toNano('0.05'));
    expect(mpDeploy.transactions).toHaveTransaction({ to: marketplace.address, deploy: true, success: true });

    const collection = blockchain.openContract(
        NftCollection.createFromConfig({
            owner: deployer.address,
            nextItemIndex: 0,
            collectionContent: buildOffchainContent('https://example.com/metadata/collection.json'),
            commonContent: buildCommonContent('https://example.com/metadata/items/'),
            nftItemCode: Codes.nftItem(),
            royalty: { factor: 500, base: 10000, address: royaltyWallet.address },
        }),
    );
    const colDeploy = await collection.sendDeploy(deployer.getSender(), toNano('0.05'));
    expect(colDeploy.transactions).toHaveTransaction({ to: collection.address, deploy: true, success: true });

    const mint = await collection.sendMint(deployer.getSender(), {
        value: toNano('0.1'),
        item: { index: 0, owner: seller.address, content: buildItemContent('0.json') },
    });
    const nftAddress = await collection.getNftAddressByIndex(0);
    expect(mint.transactions).toHaveTransaction({ to: nftAddress, deploy: true, success: true });
    const nft = blockchain.openContract(NftItem.createFromAddress(nftAddress));
    const data = await nft.getNftData();
    expect(data.owner?.equals(seller.address)).toBe(true);

    return { blockchain, deployer, seller, buyer, feeWallet, royaltyWallet, marketplaceKeys, marketplace, collection, nft };
}

function expectApprox(actual: bigint, expected: bigint, tolerance = toNano('0.001')): void {
    const diff = actual > expected ? actual - expected : expected - actual;
    expect(diff).toBeLessThanOrEqual(tolerance);
}

function buildSale(f: Fixture): NftFixpriceSaleV3R3 {
    return NftFixpriceSaleV3R3.createFromConfig({
        marketplace: f.marketplace.address,
        nft: f.nft.address,
        fullPrice: PRICE,
        marketplaceFeeAddress: f.feeWallet.address,
        marketplaceFee: MARKETPLACE_FEE,
        royaltyAddress: f.royaltyWallet.address,
        royaltyAmount: ROYALTY,
        createdAt: 1_700_000_000,
    });
}

/** Mirrors what the backend does for POST /api/listings/intent + what the wallet sends (2 messages). */
async function listNft(f: Fixture): Promise<SandboxContract<NftFixpriceSaleV3R3>> {
    const sale = f.blockchain.openContract(buildSale(f));
    const stateInit = sale.stateInitCell();

    const deployBody = buildSignedDeployBody(stateInit, buildAcceptCoinsBody(0), f.marketplaceKeys.secretKey);
    const deployResult = await f.seller.send({ to: f.marketplace.address, value: DEPLOY_VALUE, body: deployBody });
    expect(deployResult.transactions).toHaveTransaction({ from: f.seller.address, to: f.marketplace.address, success: true });
    expect(deployResult.transactions).toHaveTransaction({ from: f.marketplace.address, to: sale.address, deploy: true, success: true });

    const transferResult = await f.nft.sendTransfer(f.seller.getSender(), {
        value: TRANSFER_VALUE,
        newOwner: sale.address,
        responseDestination: f.seller.address,
        forwardAmount: FORWARD_AMOUNT,
    });
    expect(transferResult.transactions).toHaveTransaction({ from: f.nft.address, to: sale.address, success: true });
    return sale;
}

describe('Getgems fixprice sale v3r3 via marketplace v2', () => {
    let f: Fixture;

    beforeEach(async () => {
        f = await setupFixture();
    });

    it('lists an NFT: sale is deployed by the marketplace and initialized by ownership_assigned', async () => {
        const sale = await listNft(f);

        const nftData = await f.nft.getNftData();
        expect(nftData.owner?.equals(sale.address)).toBe(true);

        const saleData = await sale.getFixPriceData();
        expect(saleData.isComplete).toBe(false);
        expect(saleData.nftOwner?.equals(f.seller.address)).toBe(true);
        expect(saleData.fullPrice).toBe(PRICE);
        expect(saleData.marketplace.equals(f.marketplace.address)).toBe(true);
        expect(saleData.nft.equals(f.nft.address)).toBe(true);
    });

    it('buys a listed NFT: buyer becomes owner, seller receives price minus fee and royalty', async () => {
        const sale = await listNft(f);
        const sellerBefore = await f.seller.getBalance();
        const feeBefore = await f.feeWallet.getBalance();
        const royaltyBefore = await f.royaltyWallet.getBalance();
        const buyerBefore = await f.buyer.getBalance();

        const result = await sale.sendBuy(f.buyer.getSender(), { price: PRICE, extraGas: toNano('0.15'), queryId: 42 });
        expect(result.transactions).toHaveTransaction({ from: f.buyer.address, to: sale.address, success: true });
        expect(result.transactions).toHaveTransaction({ from: sale.address, to: f.nft.address, success: true });

        const nftData = await f.nft.getNftData();
        expect(nftData.owner?.equals(f.buyer.address)).toBe(true);

        const saleData = await sale.getFixPriceData();
        expect(saleData.isComplete).toBe(true);
        expect(saleData.queryId).toBe(42n);

        const sellerDelta = (await f.seller.getBalance()) - sellerBefore;
        const expectedSeller = PRICE - MARKETPLACE_FEE - ROYALTY;
        // Seller also receives the sale contract's leftover balance; allow a small tolerance.
        expect(sellerDelta).toBeGreaterThanOrEqual(expectedSeller);
        expect(sellerDelta).toBeLessThan(expectedSeller + toNano('0.1'));
        // Receivers pay the storage fee of their own account, so allow a tiny tolerance.
        expectApprox((await f.feeWallet.getBalance()) - feeBefore, MARKETPLACE_FEE);
        expectApprox((await f.royaltyWallet.getBalance()) - royaltyBefore, ROYALTY);

        // Buyer paid price + gas; excess gas comes back via NFT response_destination.
        const buyerSpent = buyerBefore - (await f.buyer.getBalance());
        expect(buyerSpent).toBeGreaterThan(PRICE);
        expect(buyerSpent).toBeLessThan(PRICE + toNano('0.15'));
    });

    it('rejects a buy below price + min gas', async () => {
        const sale = await listNft(f);
        const result = await f.buyer.send({ to: sale.address, value: PRICE + SALE_MIN_GAS - 1n, body: beginCell().endCell() });
        expect(result.transactions).toHaveTransaction({ from: f.buyer.address, to: sale.address, success: false, exitCode: 450 });
        const nftData = await f.nft.getNftData();
        expect(nftData.owner?.equals(sale.address)).toBe(true);
    });

    it('cancels a listing: NFT returns to the seller and the sale completes', async () => {
        const sale = await listNft(f);
        const result = await sale.sendCancel(f.seller.getSender(), { queryId: 7 });
        expect(result.transactions).toHaveTransaction({ from: f.seller.address, to: sale.address, success: true });
        expect(result.transactions).toHaveTransaction({ from: sale.address, to: f.nft.address, success: true });

        const nftData = await f.nft.getNftData();
        expect(nftData.owner?.equals(f.seller.address)).toBe(true);
        expect((await sale.getFixPriceData()).isComplete).toBe(true);
    });

    it('rejects cancel from a stranger', async () => {
        const sale = await listNft(f);
        const result = await sale.sendCancel(f.buyer.getSender());
        expect(result.transactions).toHaveTransaction({ from: f.buyer.address, to: sale.address, success: false, exitCode: 458 });
    });

    it('rejects direct sale deploy from a non-marketplace sender (exit 500)', async () => {
        const sale = f.blockchain.openContract(buildSale(f));
        const result = await f.seller.send({
            to: sale.address,
            value: DEPLOY_VALUE,
            init: sale.init,
            body: beginCell().storeUint(0xdeadbeef, 32).storeUint(0, 64).endCell(),
        });
        expect(result.transactions).toHaveTransaction({ from: f.seller.address, to: sale.address, success: false, exitCode: 500 });
    });

    it('rejects a deploy with a bad marketplace signature (exit 35)', async () => {
        const sale = f.blockchain.openContract(buildSale(f));
        const wrongKeys = keyPairFromSeed(await getSecureRandomBytes(32));
        const body = buildSignedDeployBody(sale.stateInitCell(), buildAcceptCoinsBody(), wrongKeys.secretKey);
        const result = await f.seller.send({ to: f.marketplace.address, value: DEPLOY_VALUE, body });
        expect(result.transactions).toHaveTransaction({ from: f.seller.address, to: f.marketplace.address, success: false, exitCode: 35 });
    });

    it('computes the same sale address as contractAddress(stateInit)', async () => {
        const sale = buildSale(f);
        const stateInit = sale.stateInitCell();
        const hash = stateInit.hash();
        const expected = new Address(0, hash);
        expect(sale.address.equals(expected)).toBe(true);
        expect(Cell.fromBoc(stateInit.toBoc())[0].hash().equals(hash)).toBe(true);
    });
});
