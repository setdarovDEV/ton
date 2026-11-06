import { Cell } from '@ton/core';

import collectionCompiled from '../build/NftCollection.compiled.json';
import itemCompiled from '../build/NftItem.compiled.json';
import saleCompiled from '../build/NftFixpriceSaleV3R3.compiled.json';
import marketplaceCompiled from '../build/NftMarketplaceV2.compiled.json';

/** Compiled code cells, produced by `npx blueprint build --all`. */
function cellFromCompiled(compiled: { hex: string }): Cell {
    return Cell.fromBoc(Buffer.from(compiled.hex, 'hex'))[0];
}

export const Codes = {
    nftCollection: () => cellFromCompiled(collectionCompiled),
    nftItem: () => cellFromCompiled(itemCompiled),
    fixpriceSaleV3R3: () => cellFromCompiled(saleCompiled),
    marketplaceV2: () => cellFromCompiled(marketplaceCompiled),
};
