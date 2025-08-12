# Counter contract from [IBC Apps](https://github.com/cosmos/ibc-apps/tree/26f3ad8f58e4ffc7769c6766cb42b954181dc100/modules/ibc-hooks)

This contract is a modification of the standard cosmwasm `counter` contract.
Namely, it tracks a counter, _by sender_.
This is a better way to test wasmcallbacks.
