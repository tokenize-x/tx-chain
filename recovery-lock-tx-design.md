# Recovery lock for TX: design

See [LOCK-TX.md](LOCK-TX.md) for the implementation reference, transaction path table and release staging commands. The base is v8.0.0; rebase onto the release mainnet runs before deployment.

## 1. Problem

The legitimate owners and an attacker can both sign for the same wallets. Signature validity alone cannot identify the recovery operator. Liquid balances can move immediately. Unbonding principal is exposed when it matures, and pre-existing authorizations can act without a new signature from the listed wallet.

## 2. Goal

Give the recovery operator a time-bounded window to preserve or recover the value of 191 wallets. The listed wallet and the registered fee payer co-sign every recovery transaction during the window. Where an exit transfers value out of a listed wallet, the destination is the registered recovery address.

## 3. How it is deployed

Validators coordinate a halt at height H. The old binary stores H and stops applying blocks. Validators install the lock binary, clear the halt setting and replay H with the new rules. More than two thirds of voting power must resume. An old node may follow until a block changes execution results. The adoption check deliberately distinguishes the old and new behavior.

The recovery-lock rules apply from H until the first block at or beyond expiry. The swap adds no store or migration. The operator submits recovery transactions after adoption is confirmed.

## 4. Configuration

The committed manifest contains 191 listed addresses (67 direct wallets and 124 derived addresses with non-zero holdings) and the recovery address. The enabled binary embeds two files, the list and the activation settings. TX must approve the halt height and adoption canary for release. The manifest records the confirmed address `core1mhwea2pnk7td6pt44fwl2r8vx4nhm2dcjllruq` in `feePayerAddress`. Staging sets `feePayers` to an array containing only that address. The configuration accepts 1 to 16 distinct fee payer addresses outside the listed wallet set. A fee payer may also be a recovery address. Set expiry from the activation estimate and the chain unbonding period. Release startup checks bind the exact list bytes to an expected SHA-256 and count. Plain source builds use disabled recovery settings. Tagged builds are used for rehearsals.

Apply approved changes to the `wallets` array in [app/recoverylock/mainnet-wallets.json](app/recoverylock/mainnet-wallets.json) and update the pinned count and hash in the manifest test and release documentation. Keep the approved vault, `core1uzc4cw2jtzqx8pmdqmewpt0ww3x33k9c4k59ek`, in `recoveryAddress`. The 191 entries currently produce SHA-256 `e7f2c312264a29f27ed45c826b973dc9264b952fef616f8cb7a4056c7d95ab91`. Regenerate the hash from the repository root.

```sh
jq -er '.wallets[]' app/recoverylock/mainnet-wallets.json | sha256sum
```

## 5. Rules while active

### 5.1 Coin rule and TX bank keepers

TX builds two bank keepers. The assetft and assetnft keepers use the SDK `originalBankKeeper`. The app uses wbank around a second SDK `BaseKeeper`. Its `SendCoins`, `InputOutputCoins`, and module send helpers call the assetft send hook, which moves coins through `originalBankKeeper`. This covers bank sends, fees, governance and distribution deposits, IBC escrow and outbound voucher debits, contract funds, DEX settlement, and FT commission, burn, extension, and clawback debits. The app attaches the SDK send restriction to both underlying keepers.

`DelegateCoins` (staking delegation) does not apply the send restriction. On this chain the lock does not wrap the staking bank: its only callers are the staking message server (refused for listed signers by the message rules) and the PSE module's auto-delegation, which stakes a PSE reward the module just paid into the same wallet. A guard there would make that auto-delegation fail and abort the PSE distribution, which disables PSE for the whole chain.

### 5.2 Ante and message paths

Statuses apply during the active window when a listed wallet is the signer. For operations that can name a different debit account, the reason column also covers a listed source. The ante allowlist is exhaustive. Every other message with a listed signer is blocked, including administrative messages that move no value. `TestRecoveryLockCoversEveryMessage` checks that every message type the app routes is named in the test fixtures, blocked or allowed.

| Module or path | Messages and entry points | Status | Reason |
| --- | --- | --- | --- |
| Bank send | `MsgSend` | Allowed | Recipient must be the sender's registered recovery address. The coin rule independently enforces the same destination. |
| Bank multi-send | `MsgMultiSend` | Allowed | Every output must go to the listed input's registered recovery address. |
| Bank send to any other destination | `MsgSend`, `MsgMultiSend` with any other output, including to itself | Blocked | Destination check in ante and the coin rule. |
| Bank configuration | `MsgUpdateParams`, `MsgSetSendEnabled` | Blocked | Authority operations outside the allowlist. |
| assetft tokens | `MsgSend` of an assetft denom to the recovery address | Allowed | The assetft rules still apply (issuer freezing, global freeze, recipient whitelisting). When the FT keeper charges a burn or a commission, the coin rule rejects any resulting debit to a destination other than the vault. The keeper waives these charges for a non-extension transfer involving the token admin. For a listed wallet sending an extension token, the initial debit to the extension contract is refused. |
| assetft issuer clawback | `MsgClawback` by an unlisted issuer against a listed wallet | Blocked | Debit from the listed wallet to the issuer: refused by the coin rule. Issuer freezing moves nothing and still works. |
| Fees with registered co-signer | SDK `Fee.Payer` with positive fee and empty `Fee.Granter` | Allowed | SDK verifies the payer's signature and debits the payer. The feemodel decorator requires the fee in `ucore` and at least the dynamic minimum gas price times gas, as for any transaction. |
| Fees from listed funds or grants | SDK fee payer and fee granter fields, including pre-existing fee grants | Blocked | Ante checks both fields even when every message signer is unlisted. The coin rule also prevents a debit to the fee collector. |
| Feegrant | `MsgGrantAllowance`, `MsgRevokeAllowance`, `MsgPruneAllowances` | Blocked | Outside the allowlist. |
| Authz grants and execution | `MsgGrant`, `MsgRevoke`, listed grantee's `MsgExec` | Blocked | Outside the allowlist. |
| Authz nested execution | `MsgExec` by an unlisted grantee, including recursive wrappers and grants created before activation (restake bots, nft `SendAuthorization`) | Blocked | Ante follows the inner messages and applies signer and fee rules at every depth. The authz keeper's dispatch router refuses every listed inner signer, including an otherwise permitted recovery message. Ante limits nesting to six when a listed account is involved. |
| Staking principal exit | `MsgUndelegate` | Allowed | Principal matures to the delegator. The current withdraw address must be the wallet or its recovery address. |
| Stake placement and mutation | `MsgCreateValidator`, `MsgDelegate`, `MsgBeginRedelegate`, `MsgCancelUnbondingDelegation` | Blocked | Outside the allowlist. |
| Validator controls | `MsgEditValidator`, `MsgUnjail`, staking, slashing and customparams parameter updates | Blocked | Outside the allowlist. |
| Liquid staking | Tokenize or redeem shares | Not applicable | The SDK v0.53.8 staking service has no LSM methods. |
| PSE auto-delegation | `x/pse` distribution | Unchanged | Pays a PSE reward into the delegator and stakes it to the same validators. Nothing leaves the wallet; the next undelegation covers it. |
| Distribution payouts | `MsgWithdrawDelegatorReward`, `MsgWithdrawValidatorCommission` | Allowed | Current withdraw address must be the listed account or its recovery address. |
| Withdraw address | `MsgSetWithdrawAddress` | Allowed | May point to the wallet or its recovery address. |
| Automatic staking payouts | Rewards on delegation changes, commission on validator removal, matured unbonding principal | Allowed | Staking hooks, registered before the distribution hooks, reset a listed wallet's unsafe stored withdraw address before distribution pays out. Unbonding principal goes to the delegator. |
| Distribution pool operations | `MsgFundCommunityPool`, `MsgDepositValidatorRewardsPool`, `MsgCommunityPoolSpend`, `MsgUpdateParams` | Blocked | Deposits debit the depositor. |
| Native NFT send | `cosmos.nft.v1beta1.MsgSend` to the recovery address | Allowed | The only way a native NFT leaves its owner at v8.0.0 (`x/wnft` then `x/asset/nft` `Transfer`). Class rules (soulbound, disabled sending, whitelisting, freezing) still apply. There is no coin rule for NFTs; the message rules in ante and dispatch are the full coverage. |
| Native NFT other | nft `MsgSend` to any other address, all assetnft messages (`MsgIssueClass`, `MsgMint`, `MsgBurn`, `MsgUpdateData`, freeze and whitelist messages, `MsgUpdateParams`) | Blocked | Outside the allowlist. |
| DEX cancel | `MsgCancelOrder` of the wallet's own order | Allowed | Releases the wallet's DEX locked balance and order reserve in the same account; nothing is transferred. Needed because a resting order locks the funds it offers. |
| DEX other | `MsgPlaceOrder`, `MsgCancelOrdersByDenom`, `MsgUpdateParams` | Blocked | Outside the allowlist. |
| DEX matching of a resting order | a taker crossing an order of a listed maker placed before activation | Blocked | Settlement debits the maker with `originalBankKeeper.SendCoins`; the coin rule refuses it and the taker's transaction fails. The recovery operator cancels the order. |
| assetft administration | `MsgIssue`, `MsgMint`, `MsgBurn`, freeze, global freeze, whitelist, `MsgClawback`, `MsgTransferAdmin`, `MsgClearAdmin`, DEX settings, `MsgUpdateParams` signed by a listed wallet | Blocked | A listed issuer cannot mint, claw back, move admin rights or pay an issue fee. |
| IBC ICS-20 | `MsgTransfer` over IBC v1 and v2, raw v2 `MsgSendPacket` | Blocked | A listed signer is refused in ante; the escrow or voucher burn debit is refused by the coin rule. The transfer keeper's own router (IBC v2 path) is not wrapped: it dispatches `MsgSendPacket` with the sender of a `MsgTransfer` that already passed the ante or a wrapped router, and its constructor takes the concrete `*baseapp.MsgServiceRouter`. |
| IBC client, connection, channel and relaying | All client, connection and channel messages, v1 and v2 | Blocked | No listed relayer can sign these. Ordinary relayers still deliver credits and refunds into accounts. |
| Interchain accounts | Controller `MsgRegisterInterchainAccount`, `MsgSendTx`, controller and host parameters, host `MsgModuleQuerySafe` | Blocked | A listed owner cannot drive an interchain account. The ICA host router is wrapped. |
| Packet forward, ibc-hooks, ibc-callbacks | Middleware on incoming packets | Unchanged | Credits into accounts and contract calls with derived senders, never a listed signer. |
| Governance | v1 and v1beta1 proposals, deposits, votes, cancel, legacy content, parameters | Blocked | Deposits debit the depositor; the rest is outside the allowlist. |
| Group | All group messages | Blocked | Outside the allowlist; group policy execution runs with the policy account and the group router is wrapped. |
| Vesting | `MsgCreateVestingAccount`, `MsgCreatePermanentLockedAccount`, `MsgCreatePeriodicVestingAccount` | Blocked | Debit the funding account. |
| CosmWasm exits | Fundless `MsgExecuteContract` with exact cw20 `transfer`, cw721 `transfer_nft`, `decrease_allowance`, `revoke` or `revoke_all` shape | Allowed | Transfers name the canonical recovery address. The other three remove rights. Duplicate keys, trailing JSON, unknown fields, wrong value types and optional `expires` are refused. |
| Other CosmWasm activity | Funded or other `MsgExecuteContract`, instantiate, store, migrate, admin, label, sudo, pin, instantiate config, upload params, parameters | Blocked | A listed contract admin cannot migrate or re-admin its contracts. |
| CosmWasm submessages and TX custom messages | SDK dispatch from contracts, the TX assetft, assetnft and nft encoders | Blocked for listed signers | The wasm message handler and keeper routers are wrapped. Native handlers act as the contract. |
| Other modules | auth, mint, upgrade, consensus, evidence, feemodel, customparams, PSE parameter messages | Blocked | Authority or administrative messages, outside the allowlist. |
| Delay | Delayed token upgrades and DEX GoodTil cancellations | Allowed | The handlers do not debit listed accounts. |

A listed wallet may receive coins and NFTs. Contract rights granted before activation remain effective until revoked. The contract exit checks the requested JSON shape without verifying that the contract implements cw20 or cw721 semantics.

### 5.3 Indirect execution and automatic payouts

The authz, ICA host, group and Wasm routers deny dispatch with a listed signer. Ante walks nested authz messages and limits transactions involving a listed account to six levels of nesting. Staking hooks run before distribution and reset an unsafe stored withdraw address before automatic rewards can be paid out. An already safe withdrawal address is preserved. Unbonding principal matures to the delegator. The lock deactivates by block time and requires no cleanup transaction.

## 6. Startup checks

At startup on the configured chain, enabled builds check: the list matches `listSha256` and `listCount`, is not empty and has no duplicates (compared as address bytes); every address decodes with the node's address prefix, is exactly 20 bytes and is not a module account; one recovery address per listed wallet; fee payer rules above; `chainId` is set. A build without the tag `recoverylock_test` (every release build and every plain `go test`) also requires: `chainId` `coreum-mainnet-1`; `testValues` false and none of the known test addresses (the public placeholder keys of `testdata/`); `unbondingSeconds` between 604800 (7 days, the coreum-mainnet-1 value) and one year; a mainnet reference block (`estimateRefHeight`, `estimateRefTime`) below the activation height with an implied block time between 0.3 s and 5 s up to `activationTimeEstimate`; and `activationTimeEstimate + 2 x unbondingSeconds + 7 days <= expiryTime <= activationTimeEstimate + 365 days`. Two unbonding periods because the attacker can fill all 7 unbonding entries of a delegation before the halt: the recovery operator's undelegation can then only start when the first of those entries completes, up to one period after activation, and completes one period later. Any failed check panics at startup with `recovery lock config: <reason>`, so a node with a bad list does not start.

Estimate method for a release: take the latest block (height h0, time t0; these are `estimateRefHeight` and `estimateRefTime`) and the block 10000 heights earlier (time t1); average block time b = (t0 - t1) / 10000; estimate = t0 + (H - h0) x b rounded up to the hour; use the later of this and the planned halt time. Use the live staking `unbonding_time` for `unbondingSeconds`.

At startup an enabled binary logs `recovery lock config` with `mode` (`release` or `test`), `chain_id`, `activation_height`, `expiry_time`, `expiry_utc`, `recovery_count`, `params_sha256`, `fee_payers`, `list_count` and `list_sha256`. Publish the sha256 of both files with the release so validators can compare them with that line.

## 7. Validator runbook

### Before the halt

1. Agree on a canary account T controlled by the recovery operator, an approved vault destination for T, and a configured fee payer P. Verify that T and its vault mapping are present in the exact staged configuration. On the old binary, have T grant P an authz `SendAuthorization` limited to the canary amount and T's vault, with an expiration after the planned swap. Check the grant with `txd q authz grants <T> <P>`.
2. Publish the binary sha256 (or the commit to rebuild from) and the sha256 of `listed.txt` and `params.json`.
3. Each validator sets `halt-height = H` in `app.toml` and restarts, one validator at a time, then checks that the node is in sync.
4. Pre-sign the recovery transactions: for each listed wallet, `Fee.Payer` set to one of the fee payers, signatures of the wallet and of the fee payer, fee in `ucore` at least the current feemodel minimum gas price (`txd q feemodel min-gas-price`) times the gas limit, with headroom because the price moves with block fullness. Set withdraw addresses in their own transactions, before the undelegations and withdrawals.

### Swap at H

5. At H the node logs `halt per configuration height H` and stops applying blocks. Stop the process.
6. Install the lock binary and check its sha256. Set `halt-height = 0` (otherwise the new binary stops at H again). Start.
7. The log shows `recovery lock config ... mode=release` with the published values, then the node replays block H with the lock. The chain resumes at more than 2/3 of the voting power.

### Check right after the restart (adoption check)

8. Verify the validators adopted the lock before submitting recovery transactions. Have P sign and pay for `MsgExec{grantee P, msgs [MsgSend from T to T's recovery address, the test amount]}` with a non-zero fee and no fee granter. On the lock binary it is included and fails in execution with a log ending in `listed accounts may act only in transactions paid by a fee payer: recovery lock`. Only P's fee is charged; T is unchanged.
9. Every validator checks that its node still advances after that block. A node still on the old binary executes the inner send and stops at the next block with a wrong app hash.

### Recovering a node

- Node still on the old binary after the check: stop it, `txd rollback --home <home>` (old binary, rolls back the check block), install the lock binary, `halt-height = 0`, start.
- Validator that missed the halt (executed H on the old binary): stop it, `txd rollback --home <home>`, install the lock binary, `halt-height = 0`, start. It replays H with the lock.
- New node syncing from genesis: use the lock binary and the exact configuration adopted by validators. The lock is inactive outside its configured window.

### Abort

Only before the chain resumes on the lock binary: every node that already executed H on the lock binary runs `txd rollback --home <home>` and starts on the binary with the lock disabled and `halt-height = 0`. For a retry, set the halt height and build the lock binary with a matching `activationHeight`.

### Expiry

The first block whose header time reaches `expiryTime` turns off the recovery rules. Validators continue from the state committed during the window. Use the node recovery procedure for any node that diverged. To extend the window without another halt, have every validator install the replacement binary before the configured expiry.

### Recovery operator after the check transaction

Cancel resting DEX orders, fix withdraw addresses, withdraw rewards and commission, undelegate everything, send balances, assetft tokens and NFTs to each wallet's recovery address, remove cw20 and cw721 rights. All fees are paid by the fee payer set, with `Fee.Payer` set and both the wallet and the fee payer signing.

## 8. Testing

| Local check | Verified behavior |
| --- | --- |
| Four-validator upgrade | Validators halted on v8.0.0, resumed on the lock binary and agreed on app hashes during the window and after expiry. A listed third-party send was refused, a co-signed vault transfer committed, and an ordinary listed send committed after expiry. |
| Asset transfers | Co-signed CW721 ownership recovery succeeded. An FT extension side debit was rejected without changing either balance. |
| IBC | ICS-20 and ICA handshakes, listed-outbound refusal, inbound credits, native and voucher refunds, IBC v2, packet forwarding, and Wasm hooks and callbacks. |
| Module-triggered paths | DEX GoodTil cancellation, governance refunds, unbonding maturity, PSE auto-delegation, and automatic reward routing. |
| Indirect execution | Wasm dispatch using a pre-existing authorization was refused during the lock. App tests exercise dispatch through the ICA host router with a listed signer. |
| App and stock tests | App tests verify governance deposit burns, slashing-triggered rewards, and forced commission payouts. Selected stock tests cover bank gas, Wasm authz, IBC v2, and IBC hooks. |

## 9. Known limits

- Built and tested on v8.0.0; rebase onto the release mainnet runs and rerun the tests before use.
- A recovery transfer fails if its FT burn, commission, or extension handling debits the listed wallet to a destination other than the vault.
- Issuer clawback from a listed wallet is refused while the lock is active.
- A resting DEX order of a listed wallet cannot be filled while the lock is active; it stays on the book until the recovery operator cancels it or it expires.
- Contract rights granted before activation are not revoked by the lock; the contract exit shapes let the operator revoke them.
- Copied-mainnet-state and full release-platform verification are required after rebasing onto the TX release source.
