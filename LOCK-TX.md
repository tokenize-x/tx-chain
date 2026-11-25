# Recovery lock for coreum-mainnet-1 (TX)

Implementation reference for the TX team. Base: tx-chain v8.0.0, commit `5575eea6670b80261ab9168dc7ea0339bba0da34`, with Cosmos SDK `v0.53.8` (no fork), CometBFT `v0.38.23`, Wasmd `v0.60.9` and IBC Go `v10.5.1`.

## Built on v8.0.0

This lock is built and tested on v8.0.0. Before deployment, rebase it onto the release that mainnet runs and rerun the tests and release verification on that base.

## What it is

A compiled allow-list for a coordinated halt-height upgrade. From `activation_height`, the wallets in the list may only move funds to their registered recovery address, in transactions whose fee is paid by one of the configured fee payer accounts. Everything else signed by or debiting a listed wallet is refused. Unlisted accounts and module accounts are unaffected. There is no new store, no migration, no upgrade plan and no change to any TX module: the logic is in the new package `app/recoverylock`, the rest is wiring.

The window is inclusive at `activation_height` and ends at the first block whose header time is at or after `expiry_time` (Unix seconds). CheckTx evaluates the next height and falls back to the last committed height plus one when its context has no height; with an unknown CheckTx block time it applies the height condition alone. Before activation and after expiry every lock component is a no-op.

The committed configuration is disabled. A binary built from the committed configuration wires no lock components. Delayed FT upgrades use the initialized asset keeper. The release uses the wallet list, recovery address and fee payer in `app/recoverylock/mainnet-wallets.json`, with activation settings agreed with TX.

A listed wallet must sign its recovery messages at the top level. The transaction must pay a positive fee from a configured fee payer and have an empty fee granter. Setting the transaction's `Fee.Payer` field makes the SDK require the fee payer's signature in addition to the message signers. A listed wallet's balance cannot fund fees, including fees for a different signer using an older allowance. Transactions that contain no listed signer, payer or granter follow the ordinary ante rules. If the current withdraw address is unsafe, confirm a separate `MsgSetWithdrawAddress` transaction before submitting a withdrawal or undelegation: ante reads the stored address before any message of the transaction runs.

The TX chain already ran a similar coordinated swap: v8.0.0 contains a height-gated emergency freeze of two addresses (`pkg/config/constant/freeze.go`, activation height 83530000, August 2026), enforced in the assetft send hook, the wbank delegate debit, an ante decorator and a PSE skip. It blocks every debit of a frozen account, with no recovery destination, fee payer or expiry. The recovery lock is separate from it and leaves it unchanged.

## Layout

| Path | Content |
| --- | --- |
| `app/recoverylock/config.go` | Embedded `listed.txt` and `params.json`, strict parsing, startup validation, the disabled state, the chain check, `MustConfig`. |
| `app/recoverylock/lock.go` | `Lock`, the active window, message checks with the authz depth bound, `ErrLocked` (codespace `recoverylock`, code 1). |
| `app/recoverylock/rules.go` | The allow-list for listed signers (bank, distribution, staking, nft send, DEX cancel, contract exits). |
| `app/recoverylock/contract.go` | Strict JSON shape check for the CosmWasm exits. |
| `app/recoverylock/components.go` | Bank send restriction, dispatch router wrapper, ante decorator. |
| `app/recoverylock/hooks.go` | Staking hooks that reset an unsafe withdraw address before automatic payouts. |
| `app/recoverylock/testbuild_on.go`, `testbuild_off.go` | Selected by the build tag `recoverylock_test` (tests and local test chains only): the `testBuild` constant and, in tagged builds only, `UseTestConfig`. Untagged builds contain no configuration override. |
| `app/recoverylock/mainnet-wallets.json` | Release input with the 191 affected wallets, the recovery address and the fee payer. |
| `app/recoverylock/listed.txt`, `params.json` | The committed configuration: disabled, no entries. |
| `app/recoverylock/testdata/` | Test values for the devnet chain id and `devcore` prefix (3 listed wallets, 1 recovery address, fixture fee payers, public placeholder keys) used by the tests and for local chains. |
| `app/app.go`, `x/auth/ante/ante.go` | Wiring (see Wiring). |
| `build/tx-chain/tests.go` | `make test` also runs `./app/` and `./app/recoverylock/` with `-tags recoverylock_test`. |
| `app/recoverylock/config_test.go`, `config_tag_test.go`, `app/recoverylock_test.go` | Unit and app tests, benchmarks. `config_test.go` runs in a plain `go test ./...`; the other two need `-tags recoverylock_test`. |

## Configuration

`app/recoverylock/listed.txt` holds one listed account per line. Blank lines and lines starting with `#` (after whitespace) are ignored. `app/recoverylock/params.json` is parsed strictly: unknown, duplicate or wrong-case keys, wrong value types and trailing data are refused.

Committed (disabled) state:

```json
{
  "enabled": false
}
```

with a `listed.txt` that has no entries (comments only). `"enabled": false` requires every other field to be absent and the list to be empty, so a real list with `enabled` left false makes the node refuse to start instead of running without the lock. With the disabled configuration `MustConfig` returns nil, logs `recovery lock disabled params_sha256=...`, and the app wires no lock component: no ante decorator, no send restriction, no router wrapper, no staking hooks.

The deployment manifest contains 191 listed addresses (67 direct wallets and 124 derived addresses with non-zero holdings). It also records the recovery address, `core1uzc4cw2jtzqx8pmdqmewpt0ww3x33k9c4k59ek`. Apply approved changes to its `wallets` array and update the pinned count and hash in the manifest test and release documentation. Run `jq -er '.wallets[]' app/recoverylock/mainnet-wallets.json | sha256sum` from the repository root to regenerate its release hash. TX must agree on the activation height before staging the embedded files. The manifest records `core1mhwea2pnk7td6pt44fwl2r8vx4nhm2dcjllruq` as the sole release fee payer.

Stage the embedded files in a release worktree from the repository root. Set `SETTINGS` to the path to the approved JSON object containing `activation_height`, `expiry_time`, `activation_time_estimate`, `estimate_ref_height`, `estimate_ref_time` and `unbonding_seconds`. For the committed manifest, the staging commands produce 191 lines with list SHA-256 `e7f2c312264a29f27ed45c826b973dc9264b952fef616f8cb7a4056c7d95ab91`. Validate the settings in an untagged binary before publishing the release hashes.

```bash
lock_dir=app/recoverylock
manifest="$lock_dir/mainnet-wallets.json"
jq -er '.wallets[]' "$manifest" > "$lock_dir/listed.txt"
list_hash=$(sha256sum "$lock_dir/listed.txt" | cut -d ' ' -f 1)
jq --slurpfile manifest "$manifest" --arg hash "$list_hash" '
  $manifest[0] as $m
  | .enabled = true
  | .chain_id = $m.chain_id
  | .list_count = ($m.wallets | length)
  | .list_sha256 = $hash
  | .recovery_addresses = (reduce $m.wallets[] as $wallet ({}; .[$wallet] = $m.recovery_address))
  | .fee_payers = [$m.fee_payer_address]
  | .test_values = false
' "$SETTINGS" > "$lock_dir/params.json"
sha256sum "$lock_dir/listed.txt" "$lock_dir/params.json"
```

Enabled state fields:

| Field | Meaning |
| --- | --- |
| `enabled` | `true` |
| `chain_id` | The chain the list belongs to. `txd` serves mainnet, testnet and devnet with different address prefixes (`core`, `testcore`, `devcore`); a node of another chain logs `recovery lock not wired: configured for another chain` and runs without the lock. Release builds require `coreum-mainnet-1`. |
| `recovery_addresses` | Object keyed by the canonical listed address, one recovery address per listed wallet. Several wallets may share a recovery address. A recovery address cannot be a listed wallet. |
| `fee_payers` | 1 to 16 distinct addresses. A fee payer may also be a recovery address and must be outside the listed wallet set. |
| `activation_height` | First height with the lock (the halt height H). |
| `expiry_time` | Unix seconds. The lock is off from the first block with header time at or after it. Release builds: at most one year after `activation_time_estimate`. |
| `list_count`, `list_sha256` | Count and sha256 of the raw bytes of `listed.txt`. |
| `test_values` | Must be `false` in a release build. |
| `activation_time_estimate`, `estimate_ref_height`, `estimate_ref_time`, `unbonding_seconds` | Release cross-checks, below. |

Every build checks at startup: the list matches `list_sha256` and `list_count`, is not empty and has no duplicates (compared as address bytes); every address decodes with the node's address prefix, is exactly 20 bytes and is not a module account; one recovery address per listed wallet; fee payer rules above; `chain_id` is set. A build without the tag `recoverylock_test` (every release build and every plain `go test`) also requires: `chain_id` `coreum-mainnet-1`; `test_values` false and none of the known test addresses (the public placeholder keys of `testdata/`); `unbonding_seconds` between 604800 (7 days, the coreum-mainnet-1 value) and one year; a mainnet reference block (`estimate_ref_height`, `estimate_ref_time`) below the activation height with an implied block time between 0.3 s and 5 s up to `activation_time_estimate` (mainnet: about 0.7 s, for example heights 95053554 to 95063554 took 7014 s); and `activation_time_estimate + 2 x unbonding_seconds + 7 days <= expiry_time <= activation_time_estimate + 365 days`. Two unbonding periods because the attacker can fill all 7 unbonding entries of a delegation before the halt: the recovery operator's undelegation can then only start when the first of those entries completes, up to one period after activation, and completes one period later. Any failed check panics at startup with `recovery lock config: <reason>`, so a node with a bad list does not start.

Estimate method for a release: take the latest block (height h0, time t0; these are `estimate_ref_height` and `estimate_ref_time`) and the block 10000 heights earlier (time t1); average block time b = (t0 - t1) / 10000; estimate = t0 + (H - h0) x b rounded up to the hour; use the later of this and the planned halt time. Use the live staking `unbonding_time` for `unbonding_seconds`.

At startup an enabled binary logs `recovery lock config` with `mode` (`release` or `test`), `chain_id`, `activation_height`, `expiry_time`, `expiry_utc`, `recovery_count`, `params_sha256`, `fee_payers`, `list_count` and `list_sha256`. Publish the sha256 of both files with the release so validators can compare them with that line.

CI, plain builds and untagged tests use the disabled recovery configuration. Tests that need the lock are built with `-tags recoverylock_test` and inject the files of `app/recoverylock/testdata/` with `recoverylock.UseTestConfig`, which exists only in builds with that tag. A release replaces `listed.txt` and `params.json` with the real data and is built with the normal release process and no extra tag; that build refuses test values, the known test addresses and any chain id other than `coreum-mainnet-1`.

## Two bank keepers

TX builds two bank keepers. `originalBankKeeper` (a plain SDK `BaseKeeper`) is held by the assetft and assetnft keepers. `app.BankKeeper` is the wbank wrapper around a second `BaseKeeper`; its `SendCoins`, `InputOutputCoins` and the module send helpers call the assetft send hook, which moves every coin with `originalBankKeeper`. So every account debit through the bank (bank sends, fees, gov and distribution deposits, IBC escrow and voucher burns, contract funds, DEX settlement, assetft send commission, burn rate, extension transfer and clawback) ends in `originalBankKeeper.SendCoins` or `InputOutputCoins`, where the SDK applies the send restriction. Each `BaseKeeper` has its own restriction, so the coin rule is appended to `originalBankKeeper` and also to the keeper inside the wrapper (no call path to it was found, it is defense in depth). A debit is checked once.

`DelegateCoins` (staking delegation) does not apply the send restriction. On this chain the lock does not wrap the staking bank: its only callers are the staking message server (refused for listed signers by the message rules) and the PSE module's auto-delegation, which stakes a PSE reward the module just paid into the same wallet. A guard there would make that auto-delegation fail and abort the PSE distribution, which disables PSE for the whole chain (the case TX handled for its own freeze in `x/pse/keeper/distribute.go`). The next mainnet PSE distribution is id 9 at 2027-12-04 UTC.

## Message paths

Statuses apply during the active window when a listed wallet is the signer. For operations that can name a different debit account, the reason column also covers a listed source. The ante allowlist is exhaustive. Every other message with a listed signer is blocked, including administrative messages that move no value. `TestRecoveryLockCoversEveryMessage` checks that every message type the app routes is named in the test fixtures, blocked or allowed.

| Module or path | Messages and entry points | Status | Reason |
| --- | --- | --- | --- |
| Bank send | `MsgSend` | Allowed | Recipient must be the sender's registered recovery address. The coin rule independently enforces the same destination. |
| Bank multi-send | `MsgMultiSend` | Allowed | Every output must go to the listed input's registered recovery address. |
| Bank send to any other destination | `MsgSend`, `MsgMultiSend` with any other output, including to itself | Blocked | Destination check in ante and the coin rule. |
| Bank configuration | `MsgUpdateParams`, `MsgSetSendEnabled` | Blocked | Authority operations outside the allowlist. |
| assetft tokens | `MsgSend` of an assetft denom to the recovery address | Allowed | The assetft rules still apply (issuer freezing, global freeze, recipient whitelisting). A token with a burn rate or send commission also debits the sender for the burn and for the issuer; the coin rule refuses those debits, so such a token cannot be moved while the lock is active (limit). A token with an extension sends the amount to its extension contract first; refused the same way (limit). |
| assetft issuer clawback | `MsgClawback` by an unlisted issuer against a listed wallet | Blocked | Debit from the listed wallet to the issuer: refused by the coin rule. Issuer freezing moves nothing and still works. |
| Fees with registered co-signer | SDK `Fee.Payer` with positive fee and empty `Fee.Granter` | Allowed | SDK verifies the payer's signature and debits the payer. The feemodel decorator requires the fee in `ucore` and at least the dynamic minimum gas price times gas, as for any transaction. |
| Fees from listed funds or grants | SDK fee payer and fee granter fields, including pre-existing fee grants | Blocked | Ante checks both fields even when every message signer is unlisted. The coin rule also prevents a debit to the fee collector. |
| Feegrant | `MsgGrantAllowance`, `MsgRevokeAllowance`, `MsgPruneAllowances` | Blocked | Outside the allowlist. |
| Authz grants and execution | `MsgGrant`, `MsgRevoke`, listed grantee's `MsgExec` | Blocked | Outside the allowlist. |
| Authz nested execution | `MsgExec` by an unlisted grantee, including recursive wrappers and grants created before activation (restake bots, nft `SendAuthorization`) | Blocked | Ante follows the inner messages and applies signer and fee rules at every depth. The authz keeper's dispatch router refuses every listed inner signer, including an otherwise permitted recovery message. Ante limits nesting to six when the transaction involves a listed signer, fee payer or fee granter. |
| Staking principal exit | `MsgUndelegate` | Allowed | Principal matures to the delegator. The current withdraw address must be the wallet or its recovery address. |
| Stake placement and mutation | `MsgCreateValidator`, `MsgDelegate`, `MsgBeginRedelegate`, `MsgCancelUnbondingDelegation` | Blocked | Outside the allowlist. |
| Validator controls | `MsgEditValidator`, `MsgUnjail`, staking, slashing and customparams parameter updates | Blocked | Outside the allowlist. |
| Liquid staking | Tokenize or redeem shares | Not applicable | The SDK v0.53.8 staking service has no LSM methods (checked: the type URLs have no handler). |
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
| Delay | delayed token upgrades and DEX GoodTil cancellations | Unchanged | Module side, no debit of a listed account. |

A listed wallet may receive coins and NFTs. Contract rights granted before activation remain effective until revoked. The contract exit checks the requested JSON shape without verifying that the contract implements cw20 or cw721 semantics.

## Wiring

- `app/app.go`, right before the authz keeper: `recoverylock.MustConfig(ChosenNetwork.ChainID(), address codec, module accounts, logger)` validates the embedded configuration and `recoverylock.New` returns nil for the disabled configuration or a list of another chain. Only when the lock is non-nil: the message router given to the authz, ICA host, group, wasm message handler and wasm keepers is wrapped (`Router`); the coin rule is appended to `originalBankKeeper` and to `app.BankKeeper` right after both exist; the recovery staking hooks are registered before the distribution, slashing and PSE hooks; the ante option `RecoveryLockDecorator` is set.
- `x/auth/ante/ante.go`: `HandlerOptions.RecoveryLockDecorator`; when it is non-nil the decorator is inserted right after the memo check, before the feemodel fee check, fee deduction and signature handling. It runs under the infinite gas meter that `SetInfiniteGasMeterDecorator` installs, so it adds no gas and does not change gas estimates.
- The original router continues to handle top-level messages and governance. Gov's router is not wrapped: proposal messages run with the governance module account as signer.

## Builds, CI and release

The root, builder and integration `go mod tidy` checks left their module files unchanged. `make test` built all 15 WASM fixtures. The builder race suite passed. The root race suite failed in the assetft CLI package after a block wait timed out. The race detector identified the Cosmos SDK test network's `LatestHeight`, and the package reached its ten-minute timeout in `TestQueryParams`. Targeted race runs of `TestWhitelistAndQueryWhitelisted` and `TestQueryParams` passed on both the lock and v8.0.0. The full-suite failure remains open.

The lint target reached the pinned Go linter after generation, protobuf checks and spelling checks. Two Go lint runs stopped at the 30 GB disk floor. The integration targets attempted on v8.0.0 stopped during tool setup. Their chain tests remain pending.

The initial release builds stopped at a mirror that returned HTTP 404. The matching vendor archives let two Linux amd64 builds proceed and exposed nondeterministic version linker flag order. After sorting the flags, two Linux amd64 builds from code commit `21f7a237072a2a2c8f16473cf2096da14281aee8` matched byte for byte with SHA-256 `a19c9bc0236ffd6a3c64f3cbb55681d981053564981dd614ea61e293734e79c9`. The four-platform release pair remains open. Separate untagged Linux binaries built with deliberately invalid configuration exited with code 2 and the expected startup panic for test values and for an invalid release chain ID. Rerun these release checks after rebasing onto the release mainnet runs.

## Operations runbook

### Before the halt

1. Recovery operator, on the current mainnet binary: For a rehearsal, add a test wallet T controlled by the recovery operator to the rehearsal list, with a recovery address the operator also controls and the test amount only. For mainnet adoption, TX and the recovery operator must agree on the canary before release configuration is signed off. Verify that the exact staged list contains T and that the staged recovery map points T to its approved vault. T grants a configured fee payer P an authz `SendAuthorization` with a spend limit of the test amount, T's recovery address as the only allowed recipient, and an expiration well after H. Check with `txd q authz grants <T> <P>`.
2. Publish the binary sha256 (or the commit to rebuild from) and the sha256 of `listed.txt` and `params.json`.
3. Each validator sets `halt-height = H` in `app.toml` and restarts, one validator at a time, then checks that the node is in sync.
4. Pre-sign the recovery transactions: for each listed wallet, `Fee.Payer` set to one of the fee payers, signatures of the wallet and of the fee payer, fee in `ucore` at least the current feemodel minimum gas price (`txd q feemodel min-gas-price`) times the gas limit, with headroom because the price moves with block fullness. Set withdraw addresses in their own transactions, before the undelegations and withdrawals.

### Swap at H

5. At H the node logs `halt per configuration height H` and stops applying blocks. Stop the process.
6. Install the lock binary and check its sha256. Set `halt-height = 0` (otherwise the new binary stops at H again). Start.
7. The log shows `recovery lock config ... mode=release` with the published values, then the node replays block H with the lock. The chain resumes at more than 2/3 of the voting power.

### Check right after the restart (adoption check)

8. A matching app hash proves nothing until the lock changes a result. Before any rescue transaction: P signs and pays `MsgExec{grantee P, msgs [MsgSend from T to T's recovery address, the test amount]}` with a non-zero fee and no fee granter. On the lock binary it is included and fails in execution with a log ending in `listed accounts may act only in transactions paid by a fee payer: recovery lock`. Only P's fee is charged; T is unchanged.
9. Every validator checks that its node still advances after that block. A node still on the old binary executes the inner send and stops at the next block with a wrong app hash.

### Recovering a node

- Node still on the old binary after the check: stop it, `txd rollback --home <home>` (old binary, rolls back the check block), install the lock binary, `halt-height = 0`, start.
- Validator that missed the halt (executed H on the old binary): stop it, `txd rollback --home <home>`, install the lock binary, `halt-height = 0`, start. It replays H with the lock.
- New node syncing from genesis: the lock binary alone executes every height with the same app hashes as before (blocks before activation and after expiry run as on the base release).

### Abort

Only before the chain resumes on the lock binary: every node that already executed H on the lock binary runs `txd rollback --home <home>` and starts on the old binary with `halt-height = 0` (or a new halt height for a second attempt). A later attempt needs a new binary whose `activation_height` is the new halt height.

### Expiry

The first block whose header time reaches `expiry_time` turns off the recovery rules. Validators continue from the state committed during the window. Use the node recovery procedure for any node that diverged. To extend the window without another halt, have every validator install the replacement binary before the configured expiry.

### Recovery operator after the check transaction

Cancel resting DEX orders, fix withdraw addresses, withdraw rewards and commission, undelegate everything, send balances, assetft tokens and NFTs to each wallet's recovery address, remove cw20 and cw721 rights. All fees are paid by the fee payer set, with `Fee.Payer` set and both the wallet and the fee payer signing.

## Tests

The untagged recoverylock package tests pass on v8.0.0. The tagged app and package tests also pass with `-race`. The tagged app suite classifies every registered message and exercises signed delivery, staking, assetft, DEX, NFT and IBC wiring. Four earlier wiring mutants (no coin rule on either bank keeper, no router wrap, no staking hooks) each failed the app tests. The current startup regression test fails if an invalid enabled release chain configuration is silently skipped. The unit suite also checks that a known test address re-encoded with the mainnet prefix is refused.

The four-validator devnet rehearsal halted at block 24, rolled a validator that missed the halt back to 23 and resumed on the lock binary. An adoption transaction failed in delivery at height 28, leaving the listed wallet and vault unchanged and charging the fee payer. The observer on the old binary stopped at height 29 with a wrong app hash. Listed out-of-policy bank paths were rejected at CheckTx. A co-signed recovery send succeeded at height 37, and an unlisted send succeeded at height 41. A fresh node matched the replayed block header app hash at height 42. An ordinary listed send succeeded again after expiry at height 102. Separate rehearsals exercised the abort rollback and a Cosmovisor swap with all validators committing matching state through height 26. Those two rehearsals used an expired test configuration to check the binary handling.

See [recovery-lock-tx-design.md](recovery-lock-tx-design.md) for connected ICS-20 and ICA, CW721 and module-triggered value-path results, and for remaining release checks.

## Limits

- Built and tested on v8.0.0; rebase onto the release mainnet runs and rerun the tests before use.
- assetft tokens with a burn rate, a send commission or an extension cannot be moved to the recovery address while the lock is active (the extra debits go to the burn, the issuer or the extension contract). Their issuers' own controls (freeze, clawback after expiry) are the alternative.
- Issuer clawback from a listed wallet is refused while the lock is active.
- A resting DEX order of a listed wallet cannot be filled while the lock is active; it stays on the book until the recovery operator cancels it or it expires.
- Contract rights granted before activation are not revoked by the lock; the contract exit shapes let the operator revoke them.
- The run on a copy of real mainnet state is not done yet (see the plan).
