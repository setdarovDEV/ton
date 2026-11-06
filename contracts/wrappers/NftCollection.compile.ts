import { CompilerConfig } from '@ton/blueprint';

export const compile: CompilerConfig = {
    lang: 'func',
    targets: ['contracts/imports/stdlib.fc', 'contracts/params.fc', 'contracts/op-codes.fc', 'contracts/nft-collection-editable.fc'],
};
