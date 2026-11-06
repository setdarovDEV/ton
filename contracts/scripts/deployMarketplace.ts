import { toNano } from '@ton/core';
import { NetworkProvider } from '@ton/blueprint';
import { KeyPair, keyPairFromSeed, mnemonicNew, mnemonicToPrivateKey } from '@ton/crypto';
import { NftMarketplaceV2 } from '../wrappers/NftMarketplaceV2';

/**
 * Deploys nft-marketplace-v2 (Getgems) with a signing key the backend will use to
 * authorize sale-contract deploys (POST /api/listings/intent).
 *
 * Env:
 *   MARKETPLACE_MNEMONIC  — optional 24 words; a new mnemonic is generated otherwise.
 *   MARKETPLACE_SUBWALLET — optional subwallet id (default 0).
 */
export async function run(provider: NetworkProvider): Promise<void> {
    const { keys, mnemonic } = await resolveKeys();
    const subwallet = Number(process.env.MARKETPLACE_SUBWALLET ?? 0);

    const marketplace = provider.open(
        NftMarketplaceV2.createFromConfig({ seqno: 0, subwallet, publicKey: keys.publicKey }),
    );

    const ui = provider.ui();
    ui.write(`Marketplace address: ${marketplace.address.toString({ testOnly: provider.network() !== 'mainnet' })}`);

    if (await provider.isContractDeployed(marketplace.address)) {
        ui.write('Already deployed, nothing to do.');
    } else {
        await marketplace.sendDeploy(provider.sender(), toNano('0.05'));
        await provider.waitForDeploy(marketplace.address, 30);
    }

    const seedHex = keys.secretKey.subarray(0, 32).toString('hex');
    ui.write('');
    ui.write('===== Backend configuration (store securely, never commit) =====');
    ui.write(`MARKETPLACE_ADDRESS=${marketplace.address.toRawString()}`);
    ui.write(`MARKETPLACE_PUBLIC_KEY=${keys.publicKey.toString('hex')}`);
    ui.write(`MARKETPLACE_PRIVATE_KEY=${seedHex}   # 32-byte Ed25519 seed (hex)`);
    if (mnemonic) {
        ui.write(`MARKETPLACE_MNEMONIC="${mnemonic.join(' ')}"`);
    }
    ui.write('WARNING: anyone holding MARKETPLACE_PRIVATE_KEY can authorize sale deploys through this marketplace.');
}

async function resolveKeys(): Promise<{ keys: KeyPair; mnemonic?: string[] }> {
    const fromEnv = process.env.MARKETPLACE_MNEMONIC?.trim();
    if (fromEnv) {
        const words = fromEnv.split(/\s+/);
        return { keys: await mnemonicToPrivateKey(words) };
    }
    const seedHex = process.env.MARKETPLACE_PRIVATE_KEY?.trim();
    if (seedHex) {
        return { keys: keyPairFromSeed(Buffer.from(seedHex, 'hex')) };
    }
    const mnemonic = await mnemonicNew(24);
    return { keys: await mnemonicToPrivateKey(mnemonic), mnemonic };
}
