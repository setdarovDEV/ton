import { Address, OpenedContract, toNano } from '@ton/core';
import { NetworkProvider } from '@ton/blueprint';
import { Codes } from '../wrappers/codes';
import { buildCommonContent, buildItemContent, buildOffchainContent } from '../wrappers/content';
import { MintItem, NftCollection } from '../wrappers/NftCollection';

/**
 * Deploys a TEP-62 collection (Getgems nft-collection-editable) with TEP-64 off-chain
 * metadata and batch-mints ITEMS_COUNT items to the deployer wallet.
 *
 * Env:
 *   METADATA_BASE_URL — public base URL serving web/public/metadata (e.g. https://tonnet.example.com)
 *   ITEMS_COUNT       — number of items to mint (default 8)
 *   ROYALTY_BPS       — royalty in basis points (default 500 = 5%)
 *   ROYALTY_ADDRESS   — royalty receiver (default: deployer wallet)
 */
export async function run(provider: NetworkProvider): Promise<void> {
    const ui = provider.ui();
    const baseUrl = (process.env.METADATA_BASE_URL ?? '').replace(/\/+$/, '');
    if (!baseUrl) {
        throw new Error('METADATA_BASE_URL is required, e.g. https://tonnet.example.com');
    }
    const itemsCount = Number(process.env.ITEMS_COUNT ?? 8);
    const royaltyBps = Number(process.env.ROYALTY_BPS ?? 500);

    const deployer = provider.sender().address;
    if (!deployer) {
        throw new Error('sender address is unknown; connect a wallet');
    }
    const royaltyAddress = process.env.ROYALTY_ADDRESS ? Address.parse(process.env.ROYALTY_ADDRESS) : deployer;

    const collection = provider.open(
        NftCollection.createFromConfig({
            owner: deployer,
            nextItemIndex: 0,
            collectionContent: buildOffchainContent(`${baseUrl}/metadata/collection.json`),
            commonContent: buildCommonContent(`${baseUrl}/metadata/items/`),
            nftItemCode: Codes.nftItem(),
            royalty: { factor: royaltyBps, base: 10_000, address: royaltyAddress },
        }),
    );

    const testOnly = provider.network() !== 'mainnet';
    ui.write(`Collection address: ${collection.address.toString({ testOnly })} (${collection.address.toRawString()})`);

    if (await provider.isContractDeployed(collection.address)) {
        ui.write('Collection already deployed.');
    } else {
        await collection.sendDeploy(provider.sender(), toNano('0.05'));
        await provider.waitForDeploy(collection.address, 30);
    }

    const { nextItemIndex } = await collection.getCollectionData();
    const items: MintItem[] = [];
    for (let i = nextItemIndex; i < itemsCount; i++) {
        items.push({ index: i, owner: deployer, content: buildItemContent(`${i}.json`), amount: toNano('0.05') });
    }

    if (items.length === 0) {
        ui.write(`Collection already has ${nextItemIndex} items; nothing to mint.`);
    } else {
        ui.write(`Minting ${items.length} items (${nextItemIndex}..${itemsCount - 1}) ...`);
        // 0.05 TON per item + ~0.015 TON per item for gas/forward.
        const value = toNano('0.065') * BigInt(items.length) + toNano('0.05');
        await collection.sendBatchMint(provider.sender(), { value, items });
        await waitForItems(collection, itemsCount);
    }

    ui.write('');
    ui.write('===== Backend configuration =====');
    ui.write(`COLLECTIONS=${collection.address.toRawString()}`);
    ui.write('');
    ui.write('Items:');
    for (let i = 0; i < itemsCount; i++) {
        const addr = await collection.getNftAddressByIndex(i);
        ui.write(`  #${i}: ${addr.toString({ testOnly })}  ${baseUrl}/metadata/items/${i}.json`);
    }
}

async function waitForItems(collection: OpenedContract<NftCollection>, expected: number): Promise<void> {
    for (let attempt = 0; attempt < 30; attempt++) {
        const { nextItemIndex } = await collection.getCollectionData();
        if (nextItemIndex >= expected) {
            return;
        }
        await new Promise((r) => setTimeout(r, 3000));
    }
    throw new Error('timed out waiting for mint confirmation');
}
