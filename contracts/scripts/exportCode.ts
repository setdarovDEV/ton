import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'fs';
import { join } from 'path';

/**
 * Collects compiled code from build/*.compiled.json into build/codes.json and
 * mirrors the artifacts into the Go backend (internal/chain/ton/codes) which embeds them.
 * Run after `npx blueprint build --all`.
 */
const contracts = ['NftCollection', 'NftItem', 'NftFixpriceSaleV3R3', 'NftMarketplaceV2'] as const;

const buildDir = join(__dirname, '..', 'build');
const goCodesDir = join(__dirname, '..', '..', 'internal', 'chain', 'ton', 'codes');

const codes: Record<string, { hash: string; hex: string }> = {};
for (const name of contracts) {
    const file = join(buildDir, `${name}.compiled.json`);
    if (!existsSync(file)) {
        throw new Error(`${file} missing — run "npx blueprint build --all" first`);
    }
    const compiled = JSON.parse(readFileSync(file, 'utf8')) as { hash: string; hex: string };
    codes[name] = { hash: compiled.hash, hex: compiled.hex };
    mkdirSync(goCodesDir, { recursive: true });
    copyFileSync(file, join(goCodesDir, `${name}.compiled.json`));
}

writeFileSync(join(buildDir, 'codes.json'), JSON.stringify(codes, null, 2) + '\n');
console.log(`wrote ${join(buildDir, 'codes.json')} and synced ${contracts.length} artifacts to ${goCodesDir}`);
