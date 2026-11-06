import { beginCell, Cell } from '@ton/core';

/** TEP-64 off-chain content: 0x01 prefix + snake-encoded URL. */
export function buildOffchainContent(url: string): Cell {
    return beginCell().storeUint(0x01, 8).storeStringTail(url).endCell();
}

/** Common content stored in the collection: plain snake string (no prefix) that prefixes each item's individual content. */
export function buildCommonContent(prefix: string): Cell {
    return beginCell().storeStringTail(prefix).endCell();
}

/** Individual item content appended to the common prefix, e.g. "3.json". */
export function buildItemContent(suffix: string): Cell {
    return beginCell().storeStringTail(suffix).endCell();
}

/** Parses an off-chain content cell (0x01 + snake) back to its URL. */
export function parseOffchainContent(cell: Cell): string {
    const slice = cell.beginParse();
    const prefix = slice.loadUint(8);
    if (prefix !== 0x01) {
        throw new Error(`not an off-chain content cell (prefix 0x${prefix.toString(16)})`);
    }
    return slice.loadStringTail();
}
