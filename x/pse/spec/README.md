# x/pse

## Abstract

This document specifies the `pse` module (Proof of Support Emission). The module is responsible for managing scheduled token distributions from clearing accounts to designated recipients based on a predefined allocation schedule. The Community clearing account uses a special score-based distribution mechanism that rewards stakers proportionally to their staking duration and amount. This module was introduced in the v6 upgrade of TX Blockchain.

## Concepts

The PSE module implements a sophisticated token distribution system with the following key components:

## Clearing Accounts

The PSE module manages six clearing accounts(module accounts), each serving a specific purpose in the token distribution ecosystem:

- **Community** (`pse_community`) - 40% allocation, uses score-based distribution to stakers
- **Foundation** (`pse_foundation`) - 30% allocation, direct transfers to recipients
- **Alliance** (`pse_alliance`) - 20% allocation, direct transfers to recipients
- **Partnership** (`pse_partnership`) - 3% allocation, direct transfers to recipients
- **Investors** (`pse_investors`) - 5% allocation, direct transfers to recipients
- **Team** (`pse_team`) - 2% allocation, direct transfers to recipients

All clearing accounts receive their initial allocation during the v6 upgrade and distribute tokens gradually over an 84-month period according to a predefined schedule.

## Distribution Schedule

The PSE module maintains a time-based distribution schedule that defines when and how much each clearing account should distribute. The schedule is created during the v6 upgrade and operates as follows:

- **Total Duration**: 84 months from the start date
- **Start Date**: Set to 12:00 GMT on the day of the v6 software upgrade, capped at day 28 to ensure all months can accommodate the distribution date
- **Distribution Frequency**: Monthly distributions on the same day of each month (matching the start date day, capped at 28)
- **Amount per Period**: Each clearing account distributes an equal portion (1/84) of its total allocation each month
- **Processing**: Distributions are automatically processed during the `EndBlock` phase when the scheduled timestamp is reached

The schedule is stored in ascending order by timestamp, and the module processes one distribution period at a time, ensuring predictable and transparent token releases.

## Community Distribution - Score-Based Mechanism

The Community clearing account uses a unique score-based distribution system that rewards delegators based on both **staking amount** and **staking duration**. This incentivizes long-term participation in network security.

### Score Calculation

Each delegator's score is calculated using the formula:

```text
Score = Σ (Delegated_Tokens × Time_Staked)
```

Where:

- `Delegated_Tokens` is the amount of tokens delegated to validators (in base denomination units)
- `Time_Staked` is the duration in seconds that the tokens have been staked

The score accumulates over a **1-month period** between distributions (since scores reset to zero after each monthly distribution). The score is tracked separately for each delegation to each validator. When delegations are modified (increased, decreased, or removed), the module:

1. Calculates the score earned since the last modification
2. Adds it to the delegator's account score snapshot
3. Resets the time counter for the delegation

### Score Tracking Implementation

The module maintains two key data structures for score tracking:

- **DelegationTimeEntries**: Tracks the shares and last modification timestamp for each (delegator, validator) pair
- **AccountScoreSnapshot**: Stores the accumulated score for each delegator account

### Staking Hooks Integration

The PSE module integrates with the staking module through hooks that trigger on delegation events:

- **AfterDelegationModified**: Updates the score when delegations are created or modified
- **BeforeDelegationRemoved**: Finalizes the score calculation when a delegation is completely removed

### Distribution Process for Community

When a Community distribution is scheduled:

1. **Score Finalization**: The module iterates through all active delegations and calculates any uncalculated scores (time since last change up to current block)
2. **Total Score Calculation**: Sums all delegator scores to get the total score
3. **Proportional Distribution**: Each delegator receives tokens proportional to their score:

   ```text
   Delegator_Amount = (Delegator_Score / Total_Score) × Distribution_Amount
   ```

4. **Auto-Delegation**: Distributed tokens are automatically delegated to the delegator's validators in the same proportion as their existing delegations
5. **Leftover Handling**: Any leftover from rounding errors or delegators with no active delegations is sent to the community pool
6. **Score Reset**: All scores are reset to zero for the next 1-month distribution period

### Excluded Addresses

The module maintains a list of excluded addresses that are not eligible to receive Community distributions. This list can be updated via governance and is useful for excluding exchange addresses or other entities that should not participate in the score-based distribution.

## Non-Community Distribution - Direct Transfers

For all clearing accounts except Community (Foundation, Alliance, Partnership, Investors, Team), the distribution mechanism is simpler:

1. Each clearing account has one or more mapped recipient addresses (configured via governance)
2. When a distribution is scheduled, the allocation amount is divided equally among all recipients
3. Tokens are transferred directly from the clearing account to recipient addresses
4. Any remainder from integer division is sent to the community pool

This direct transfer mechanism provides flexibility for institutional distributions while maintaining transparency through on-chain recipient mappings.

### Recipient Mappings

Recipient mappings are stored in module parameters and define which addresses receive distributions from each non-Community clearing account:

```go
ClearingAccountMapping {
    clearing_account: string      // e.g., "pse_foundation"
    recipient_addresses: []string // List of recipient addresses
}
```

- Mappings are validated at genesis and during updates
- Community clearing account cannot have recipient mappings (uses score-based distribution)
- Each non-Community clearing account must have at least one recipient
- Mappings can be updated via governance through the `UpdateClearingMappings` transaction

## State

State managed by the PSE module:

- **Params**: `0x00 | -> Params`
- **DelegationTimeEntries**: `0x01 | delegator_address | validator_address -> DelegationTimeEntry`
- **AccountScoreSnapshot**: `0x02 | delegator_address -> Int`
- **AllocationSchedule**: `0x03 | timestamp (uint64) -> ScheduledDistribution`

### Params

Module parameters containing:

- `ExcludedAddresses`: List of addresses excluded from Community distributions
- `ClearingAccountMappings`: Recipient address mappings for non-Community clearing accounts

### DelegationTimeEntry

Tracks the last modification time and shares for each (delegator, validator) pair:

```protobuf
message DelegationTimeEntry {
  int64 last_changed_unix_sec = 1;  // Unix timestamp of last delegation change
  string shares = 2;                 // Validator shares held by delegator
}
```

### AccountScoreSnapshot

Stores the accumulated score for each delegator address over the current 1-month period. This snapshot is updated whenever:

- A delegation is modified or removed
- A Community distribution is processed (reset to zero for the next month)

### AllocationSchedule

Maps timestamps to scheduled distributions:

```protobuf
message ScheduledDistribution {
  uint64 timestamp = 1;                               // Unix timestamp when distribution should occur
  repeated ClearingAccountAllocation allocations = 2; // Allocations for each clearing account
}

message ClearingAccountAllocation {
  string clearing_account = 1;  // Clearing account module name
  string amount = 2;             // Amount to distribute (in base denomination)
}
```

## Keeper

The PSE module keeper provides functionality across five main areas:

### Distribution Processing

Handles the automatic processing of scheduled distributions during the `EndBlock` phase. For Community distributions, it finalizes all pending scores, calculates proportional allocations, and auto-delegates tokens to validators. For non-Community distributions, it transfers tokens directly to recipient addresses. Only one distribution is processed per block, ensuring predictable gas usage.

### Score Management

Manages the calculation and storage of delegator scores used in Community distributions. Tracks delegation time entries for each (delegator, validator) pair and maintains account score snapshots. Calculates both accumulated scores from snapshots and real-time uncalculated scores from active delegations since the last update.

### Schedule Management

Manages the 84-month distribution schedule stored in blockchain state. Provides methods to save, retrieve, and peek at scheduled distributions. The schedule is maintained in chronological order by timestamp, with the earliest pending distribution processed first during `EndBlock`.

### Parameter Management

Handles module parameter storage and updates, including the excluded addresses list and clearing account recipient mappings. Validates parameter changes to ensure consistency with distribution rules and supports governance-driven updates through authorized transactions.

### Query Helpers

Provides utility methods for querying module state, such as retrieving current balances of all clearing accounts. These helpers support both CLI queries and programmatic access to module data for monitoring and auditing purposes.

## Messages

### MsgUpdateExcludedAddresses

Governance-only message to update the list of addresses excluded from Community distributions.

```protobuf
message MsgUpdateExcludedAddresses {
  string authority = 1;                    // Must be governance module address
  repeated string addresses_to_add = 2;    // Addresses to add to exclusion list
  repeated string addresses_to_remove = 3; // Addresses to remove from exclusion list
}
```

**Authorization**: Only governance (`gov` module)

**Validation**:

- Authority must match the governance module address
- All addresses must be valid bech32 addresses
- No duplicates within the add or remove lists
- Addresses in remove list must currently exist in the exclusion list

**Use Cases**:

- Excluding exchange hot wallets from Community distributions
- Excluding smart contracts that shouldn't receive staking rewards
- Removing previously excluded addresses to re-enable their eligibility

## Queries

### Params Query

Query the current module parameters, including excluded addresses and clearing account mappings.

```bash
txd query pse params
```

**Response**:

```json
{
  "params": {
    "excluded_addresses": ["core1...", "core2..."],
    "clearing_account_mappings": [
      {
        "clearing_account": "pse_foundation",
        "recipient_addresses": ["core1..."]
      }
    ]
  }
}
```

### Score

Query the current total score for a specific delegator address.

```bash
txd query pse score [delegator-address]
```

**Response**:

```json
{
  "score": "1234567890"
}
```

The returned score includes:

- Accumulated score from the current 1-month period (snapshot)
- Uncalculated scores from all active delegations since the last score update

**Example**:

```bash
txd query pse score core1abc123...
```

### ClearingAccountBalances

Query the current balances of all PSE clearing accounts.

```bash
txd query pse clearing-account-balances
```

**Response**:

```json
{
  "balances": [
    {
      "clearing_account": "pse_community",
      "balance": "40000000000000000"
    },
    {
      "clearing_account": "pse_foundation",
      "balance": "30000000000000000"
    },
    {
      "clearing_account": "pse_alliance",
      "balance": "20000000000000000"
    },
    {
      "clearing_account": "pse_partnership",
      "balance": "3000000000000000"
    },
    {
      "clearing_account": "pse_investors",
      "balance": "5000000000000000"
    },
    {
      "clearing_account": "pse_team",
      "balance": "2000000000000000"
    }
  ]
}
```

This query is useful for:

- Monitoring remaining balances in each clearing account
- Verifying distributions are processing correctly
- Auditing the token distribution schedule

## Events

### EventAllocationDistributed

Emitted when a scheduled allocation is distributed from a non-Community clearing account.

```protobuf
message EventAllocationDistributed {
  string clearing_account = 1;           // Source clearing account
  repeated string recipient_addresses = 2; // List of recipients
  string amount_per_recipient = 3;       // Amount each recipient received
  string community_pool_amount = 4;      // Remainder sent to community pool
  uint64 scheduled_at = 5;               // Original scheduled timestamp
  string total_amount = 6;               // Total amount distributed
}
```

## Upgrade Handler (v6)

The PSE module is initialized during the v6 blockchain upgrade. The upgrade handler performs the following operations:

### 1. Store Upgrades

Adds the PSE module store key to the blockchain state:

```go
StoreUpgrades: store.StoreUpgrades{
    Added: []string{psetypes.StoreKey},
}
```

### 2. Initial Token Minting and Allocation

Mints 100 billion tokens (in base denomination) and distributes them to the six clearing accounts according to the allocation percentages defined in the [Clearing Accounts](#clearing-accounts) section.

### 3. Distribution Schedule Creation

Creates an 84-month distribution schedule with:

- Start date set to 12:00 GMT on the v6 upgrade day, with the day of month capped at 28
- Monthly distributions occurring on the same day of each month (capped at day 28)
- Equal allocation amounts per clearing account per month
- Timestamps calculated using Gregorian calendar month arithmetic

### 4. Clearing Account Mapping Initialization

Sets up default recipient mappings for non-Community clearing accounts (Foundation, Alliance, Partnership, Investors, and Team). Each clearing account is configured with at least one recipient address that will receive direct token distributions. These mappings are chain-specific and can be updated via governance after the upgrade.

### 5. Staking Snapshot

Captures a snapshot of all existing delegations at the upgrade block height, initializing the delegation time entries with:

- Current block timestamp as `last_changed_unix_sec`
- Current validator shares for each delegation

This ensures that stakers who delegated before the upgrade are not disadvantaged and start accumulating scores immediately.

## Parameters

The PSE module parameters can be queried but are primarily managed through governance proposals.

| Parameter                 | Type                          | Description                                                |
|---------------------------|-------------------------------|------------------------------------------------------------|
| ExcludedAddresses         | []string                      | Addresses excluded from Community score-based distribution |
| ClearingAccountMappings   | []ClearingAccountMapping      | Recipient address mappings for non-Community accounts      |

### ExcludedAddresses

- Contains bech32-encoded account addresses
- Addresses in this list will not accumulate scores or receive Community distributions
- No duplicates allowed
- Can be updated via governance using `MsgUpdateExcludedAddresses`
- Common use case: excluding exchange addresses that custody user funds

### ClearingAccountMappings

- Each non-Community clearing account must have exactly one mapping entry
- Each mapping must contain at least one recipient address
- Community clearing account (`pse_community`) cannot have a mapping (uses score-based distribution)
- All recipient addresses must be valid bech32 addresses
- No duplicate recipients within a single clearing account
- Can be updated via governance

**Validation Rules**:

- All addresses must be valid bech32 format
- Community clearing account cannot appear in mappings
- Each clearing account in the distribution schedule must have a corresponding mapping (except Community)
- No duplicate clearing accounts across mappings

## Integration with Other Modules

### Staking Module

The PSE module integrates tightly with the staking module through:

- **Staking Hooks**: Automatically updates scores when delegations change
- **Delegation Queries**: Retrieves current delegation amounts for score calculations
- **Auto-Delegation**: Distributes Community tokens by automatically delegating to validators

### Distribution Module

- **Community Pool Funding**: Sends rounding remainders and leftover tokens to the community pool
- Uses the `FundCommunityPool` method to ensure proper accounting

### Bank Module

- **Token Transfers**: All distribution transfers use the bank module's `SendCoinsFromModuleToAccount` and `SendCoinsFromModuleToModule` methods
- **Balance Queries**: Retrieves clearing account balances for monitoring

### Governance Module

- **Parameter Updates**: All parameter changes require governance proposals
- **Authority Validation**: Only the governance module can execute `MsgUpdateExcludedAddresses` and mapping updates

## Important Considerations

### Score Calculation Accuracy

The score calculation is designed to be as accurate as possible, but there are a few edge cases:

1. **Validator Slashing**: Currently, slashing events do not trigger intermediate score updates. If a validator is slashed, the delegator's score continues to accumulate based on the pre-slashing token amount until the next delegation modification or distribution. This is documented as a known limitation (TODO in the code).

2. **Rounding Errors**: Integer division during distribution may result in small rounding errors (up to 1 base unit per delegator). These remainders are sent to the community pool.

3. **Score Resets**: All scores are reset to zero after each Community distribution, ensuring a fresh start for each 1-month distribution period.

### Distribution Timing

- Distributions are processed automatically during the `EndBlock` phase
- Only one distribution is processed per block, even if multiple are overdue
- If the chain is halted and later restarted, overdue distributions will be processed sequentially in subsequent blocks
- The module processes distributions in chronological order based on timestamp

### Token Economics

The PSE module introduces 100 billion new tokens during the v6 upgrade, distributed over 84 months:

- **Monthly Release**: Approximately 1.19 billion tokens per month across all clearing accounts
- **Community Portion**: Approximately 476 million tokens per month for score-based distribution
- **Inflation Impact**: This is a one-time token addition, not continuous inflation

### Governance Responsibilities

Governance has the responsibility to:

- Update excluded addresses list as needed
- Modify recipient mappings for non-Community clearing accounts
- Ensure recipient addresses are secure and appropriate for institutional distributions
- Monitor distribution fairness and adjust parameters if needed

## Security Considerations

### Clearing Account Security

- All clearing accounts are module accounts owned by the PSE module
- Tokens can only be distributed according to the hardcoded schedule and logic
- No admin keys or manual intervention possible for fund transfers
- Distribution logic is deterministic and auditable

### Recipient Mapping Security

- Recipient addresses for non-Community accounts are stored on-chain
- Changes require governance approval
- Addresses are validated at genesis and during updates
- It's crucial to verify recipient addresses before submitting governance proposals

### Smart Contract Risk

- If a smart contract is set as a recipient, ensure it can receive and handle tokens
- Smart contracts in excluded addresses will not accumulate Community scores
- Auto-delegation of Community rewards may fail if the recipient is a contract without delegation capability
