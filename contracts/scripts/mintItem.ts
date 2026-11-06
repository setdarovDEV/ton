import { Address, toNano } from '@ton/core';
import { NetworkProvider } from '@ton/blueprint';
import { buildItemContent } from '../wrappers/content';
import { NftCollection } from '../wrappers/NftCollection';

/**
 * Mints the next item of an existing collection.
 *
 * Env:
 *   COLLECTION_ADDRESS — collection to mint into (must be owned by the sender)
 *   ITEM_OWNER         — optional recipient (default: sender)
 */
export async function run(provider: NetworkProvider): Promise<void> {
    const ui = provider.ui();
    const collectionAddress = process.env.COLLECTION_ADDRESS;
    if (!collectionAddress) {
        throw new Error('COLLECTION_ADDRESS is required');
    }
    const sender = provider.sender().address;
    if (!sender) {
        throw new Error('sender address is unknown; connect a wallet');
    }
    const owner = process.env.ITEM_OWNER ? Address.parse(process.env.ITEM_OWNER) : sender;

    const collection = provider.open(NftCollection.createFromAddress(Address.parse(collectionAddress)));
    const { nextItemIndex } = await collection.getCollectionData();

    await collection.sendMint(provider.sender(), {
        value: toNano('0.1'),
        item: { index: nextItemIndex, owner, content: buildItemContent(`${nextItemIndex}.json`), amount: toNano('0.05') },
    });

    const itemAddress = await collection.getNftAddressByIndex(nextItemIndex);
    ui.write(`Mint of item #${nextItemIndex} sent. Item address: ${itemAddress.toString({ testOnly: provider.network() !== 'mainnet' })}`);
    ui.write('Remember to add web/public/metadata/items/' + nextItemIndex + '.json');
}
