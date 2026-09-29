package v8

import (
	"context"

	sdkmath "cosmossdk.io/math"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/pkg/errors"
)

var (
	// PSEPauseInflation is the fixed inflation while PSE is postponed (mainnet proposal 46).
	// It targets a 24% APR for delegators of an average-commission validator (5.77%) after the 5% community tax.
	// It is sized on mainnet's expected bonded stake after the November 2026 distribution: 5.12B of 102.09B TX.
	// The same value applies on every network, so testnet rehearses the mainnet parameters.
	PSEPauseInflation = sdkmath.LegacyMustNewDecFromStr("0.01345")

	// PSEPauseBlocksPerYear matches mainnet's measured block time (0.746s), so the configured inflation is what is minted.
	// With the previous 33M the chain minted about 28% more than configured, and explorers under-reported the APR.
	PSEPauseBlocksPerYear uint64 = 42_300_000
)

// SetPSEPauseMintParams pins inflation to PSEPauseInflation for the PSE postponement.
// Setting both bounds to the same value keeps x/mint from drifting away from it.
// The minter is updated as well, so the new rate applies from the upgrade block.
// Reverting after the postponement is a governance MsgUpdateParams, which cannot set the minter but lets it drift back.
func SetPSEPauseMintParams(ctx context.Context, mintKeeper mintkeeper.Keeper) error {
	params, err := mintKeeper.Params.Get(ctx)
	if err != nil {
		return err
	}
	params.InflationMin = PSEPauseInflation
	params.InflationMax = PSEPauseInflation
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
	minter.Inflation = PSEPauseInflation
	if err := minttypes.ValidateMinter(minter); err != nil {
		return errors.Wrap(err, "invalid minter")
	}

	return mintKeeper.Minter.Set(ctx, minter)
}
