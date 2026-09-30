package v8

import (
	"context"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/pkg/errors"

	psekeeper "github.com/tokenize-x/tx-chain/v8/x/pse/keeper"
)

var (
	// PSEPauseTargetAPY is the staking APY targeted during the PSE pause (mainnet proposal 46).
	// APY is defined as in the TX inflation model: inflation / (bonded / total supply).
	PSEPauseTargetAPY = sdkmath.LegacyMustNewDecFromStr("0.24")

	// PSEPauseBlocksPerYear matches mainnet's block time (0.746s), so the configured inflation is what is minted.
	PSEPauseBlocksPerYear uint64 = 42_300_000
)

// PSEPauseKeepers groups the keepers needed to set the pause mint params.
type PSEPauseKeepers struct {
	PSE     psekeeper.Keeper
	Mint    mintkeeper.Keeper
	Staking StakingKeeper
	Bank    BankKeeper
}

// StakingKeeper is the staking keeper subset the pause mint params need.
type StakingKeeper interface {
	TotalBondedTokens(ctx context.Context) (sdkmath.Int, error)
}

// BankKeeper is the bank keeper subset the pause mint params need.
type BankKeeper interface {
	GetSupply(ctx context.Context, denom string) sdk.Coin
}

// PSEPauseInflation returns the inflation giving PSEPauseTargetAPY at the current stake.
// It is target x bonded / total supply.
func PSEPauseInflation(
	ctx context.Context,
	stakingKeeper StakingKeeper,
	bankKeeper BankKeeper,
	denom string,
) (sdkmath.LegacyDec, error) {
	bonded, err := stakingKeeper.TotalBondedTokens(ctx)
	if err != nil {
		return sdkmath.LegacyDec{}, err
	}
	supply := bankKeeper.GetSupply(ctx, denom).Amount
	if !supply.IsPositive() {
		return sdkmath.LegacyDec{}, errors.Errorf("no %s supply", denom)
	}

	return PSEPauseTargetAPY.MulInt(bonded).QuoInt(supply), nil
}

// SetPSEPauseMintParams fixes inflation at PSEPauseInflation (min = max = minter) and sets PSEPauseBlocksPerYear.
func SetPSEPauseMintParams(ctx context.Context, keepers PSEPauseKeepers) error {
	mintKeeper := keepers.Mint
	params, err := mintKeeper.Params.Get(ctx)
	if err != nil {
		return err
	}
	inflation, err := PSEPauseInflation(ctx, keepers.Staking, keepers.Bank, params.MintDenom)
	if err != nil {
		return err
	}
	params.InflationMin = inflation
	params.InflationMax = inflation
	params.BlocksPerYear = PSEPauseBlocksPerYear
	if err := params.Validate(); err != nil {
		return errors.Wrap(err, "invalid mint params")
	}
	if err := mintKeeper.Params.Set(ctx, params); err != nil {
		return err
	}

	minter, err := mintKeeper.Minter.Get(ctx)
	if err != nil {
		return err
	}
	minter.Inflation = inflation
	if err := minttypes.ValidateMinter(minter); err != nil {
		return errors.Wrap(err, "invalid minter")
	}
	if err := mintKeeper.Minter.Set(ctx, minter); err != nil {
		return err
	}

	sdk.UnwrapSDKContext(ctx).Logger().Info("PSE pause mint params set", "inflation", inflation.String())
	return nil
}

// LastProcessedPSEDistributionID returns the ID of the last completed PSE distribution, or 0 when there is none.
func LastProcessedPSEDistributionID(ctx context.Context, pseKeeper psekeeper.Keeper) (uint64, error) {
	id, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return 0, nil
	}

	return id, err
}

// ApplyPSEPauseMintParams sets the pause mint params once, in the block the last pre-pause distribution completes.
// Errors are only logged, so the chain never halts.
func ApplyPSEPauseMintParams(
	ctx sdk.Context,
	keepers PSEPauseKeepers,
	lastProcessedBefore uint64,
) {
	if err := applyPSEPauseMintParams(ctx, keepers, lastProcessedBefore); err != nil {
		ctx.Logger().Error("PSE pause mint params failed", "error", err)
	}
}

// applyPSEPauseMintParams sets the params when the distribution completed in this block is the last pre-pause one.
func applyPSEPauseMintParams(ctx sdk.Context, keepers PSEPauseKeepers, lastProcessedBefore uint64) error {
	lastProcessed, err := LastProcessedPSEDistributionID(ctx, keepers.PSE)
	if err != nil {
		return err
	}
	// No distribution completed in this block, which is the normal case.
	if lastProcessed == lastProcessedBefore {
		return nil
	}

	last, err := isLastPrePauseDistribution(ctx, keepers.PSE, lastProcessed)
	if err != nil {
		return err
	}
	// A distribution completed, but not the last one before the pause.
	if !last {
		return nil
	}

	return SetPSEPauseMintParams(ctx, keepers)
}

// isLastPrePauseDistribution reports whether id is the last distribution on or before PSEPostponeCutoff.
// The next distribution must exist and be after the cutoff, so schedules without a pause never match.
func isLastPrePauseDistribution(ctx context.Context, pseKeeper psekeeper.Keeper, id uint64) (bool, error) {
	current, err := pseKeeper.AllocationSchedule.Get(ctx, id)
	if errors.Is(err, collections.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	next, err := pseKeeper.AllocationSchedule.Get(ctx, id+1)
	if errors.Is(err, collections.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	cutoff := uint64(PSEPostponeCutoff.Unix())
	return current.Timestamp <= cutoff && next.Timestamp > cutoff, nil
}
