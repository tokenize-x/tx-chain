package v7patch1

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/pkg/config"
	"github.com/tokenize-x/tx-chain/v7/x/pse"
	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

func accountBech32Prefix() string {
	return sdk.GetConfig().GetBech32AccountAddrPrefix()
}

func validatorBech32Prefix() string {
	return sdk.GetConfig().GetBech32ValidatorAddrPrefix()
}

func setup(t *testing.T) (sdk.Context, pskeeper.Keeper) {
	t.Helper()

	key := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	cms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, db)
	require.NoError(t, cms.LoadLatestVersion())

	ctx := sdk.NewContext(cms, tmproto.Header{}, false, log.NewNopLogger())
	encodingConfig := config.NewEncodingConfig(pse.AppModuleBasic{})
	storeService := runtime.NewKVStoreService(key)

	addressCodec := authcodec.NewBech32Codec(accountBech32Prefix())
	valAddressCodec := authcodec.NewBech32Codec(validatorBech32Prefix())

	keeper := pskeeper.NewKeeper(
		storeService,
		encodingConfig.Codec,
		"", // authority
		nil, nil, nil, nil,
		addressCodec,
		valAddressCodec,
	)

	return ctx, keeper
}

// seedBrokenState mirrors the testnet state after the failed block 85896300:
// snapshot entries present, TotalScore empty, ongoing distribution set, disabled.
func seedBrokenState(
	t *testing.T,
	ctx sdk.Context,
	keeper pskeeper.Keeper,
	distID uint64,
	entries []struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	},
	excludedAddrs []string,
) {
	t.Helper()
	requireT := require.New(t)

	for _, e := range entries {
		requireT.NoError(keeper.AccountScoreSnapshot.Set(
			ctx, collections.Join(distID, e.addr), e.score,
		))
	}

	requireT.NoError(keeper.OngoingDistribution.Set(ctx, types.ScheduledDistribution{
		ID:        distID,
		Timestamp: 1776772800,
	}))

	requireT.NoError(keeper.DistributionDisabled.Set(ctx, true))

	params := types.DefaultParams()
	params.ExcludedAddresses = excludedAddrs
	requireT.NoError(keeper.SetParams(ctx, params))
}

// TestRecoverOngoingDistribution_HappyPath reproduces the testnet broken
// state (5 non-zero stranded + 2 zero snapshot entries, empty TotalScore,
// disabled=true) and asserts the recovery restores TotalScore to the exact
// observed sum and clears the flag.
func TestRecoverOngoingDistribution_HappyPath(t *testing.T) {
	requireT := require.New(t)
	ctx, keeper := setup(t)

	const distID = uint64(3)

	type seed = struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}

	nonZero := []seed{
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(2_199_878_160_334_053)},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(2_003_391_705_000_000)},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(595_409_231_313_825)},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(8_819_999_720)},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(1_749_486_060)},
	}
	zeros := []seed{
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.ZeroInt()},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.ZeroInt()},
	}

	seedBrokenState(t, ctx, keeper, distID, append(append([]seed{}, nonZero...), zeros...), nil)

	_, err := keeper.TotalScore.Get(ctx, distID)
	requireT.ErrorIs(err, collections.ErrNotFound)

	disabled, err := keeper.DistributionDisabled.Get(ctx)
	requireT.NoError(err)
	requireT.True(disabled)

	addressCodec := authcodec.NewBech32Codec(accountBech32Prefix())
	requireT.NoError(RecoverOngoingDistribution(ctx, keeper, addressCodec))

	expectedSum := sdkmath.ZeroInt()
	for _, e := range nonZero {
		expectedSum = expectedSum.Add(e.score)
	}
	requireT.Equal("4798689666133658", expectedSum.String())

	gotTotal, err := keeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)
	requireT.True(expectedSum.Equal(gotTotal))

	disabled, err = keeper.DistributionDisabled.Get(ctx)
	requireT.NoError(err)
	requireT.False(disabled)

	// OngoingDistribution must remain; next EndBlock resumes the distribution.
	ongoing, err := keeper.OngoingDistribution.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(distID, ongoing.ID)

	for _, e := range nonZero {
		got, err := keeper.AccountScoreSnapshot.Get(ctx, collections.Join(distID, e.addr))
		requireT.NoError(err)
		requireT.True(e.score.Equal(got))
	}
	for _, e := range zeros {
		got, err := keeper.AccountScoreSnapshot.Get(ctx, collections.Join(distID, e.addr))
		requireT.NoError(err)
		requireT.True(got.IsZero())
	}

	excludedCount := 0
	excIter, err := keeper.ExcludedAddressScore.Iterate(ctx, nil)
	requireT.NoError(err)
	for ; excIter.Valid(); excIter.Next() {
		excludedCount++
	}
	excIter.Close()
	requireT.Equal(0, excludedCount)
}

// TestRecoverOngoingDistribution_MovesExcludedEntries asserts that snapshot
// entries for excluded addresses are routed to ExcludedAddressScore and
// excluded from the restored TotalScore.
func TestRecoverOngoingDistribution_MovesExcludedEntries(t *testing.T) {
	requireT := require.New(t)
	ctx, keeper := setup(t)

	addressCodec := authcodec.NewBech32Codec(accountBech32Prefix())
	const distID = uint64(3)

	normal := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	excludedAddr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	excludedBech32, err := addressCodec.BytesToString(excludedAddr)
	requireT.NoError(err)

	type seed = struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}

	normalScore := sdkmath.NewInt(1_000_000_000)
	excludedScore := sdkmath.NewInt(500_000_000)

	seedBrokenState(t, ctx, keeper, distID, []seed{
		{normal, normalScore},
		{excludedAddr, excludedScore},
	}, []string{excludedBech32})

	requireT.NoError(RecoverOngoingDistribution(ctx, keeper, addressCodec))

	gotTotal, err := keeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)
	requireT.True(normalScore.Equal(gotTotal))

	_, err = keeper.AccountScoreSnapshot.Get(ctx, collections.Join(distID, excludedAddr))
	requireT.ErrorIs(err, collections.ErrNotFound)

	gotNormal, err := keeper.AccountScoreSnapshot.Get(ctx, collections.Join(distID, normal))
	requireT.NoError(err)
	requireT.True(normalScore.Equal(gotNormal))

	gotExcluded, err := keeper.ExcludedAddressScore.Get(ctx, excludedAddr)
	requireT.NoError(err)
	requireT.True(excludedScore.Equal(gotExcluded))
}

// TestRecoverOngoingDistribution_NoOngoingIsNoOp asserts the recovery is a
// safe no-op on chains without an ongoing distribution.
func TestRecoverOngoingDistribution_NoOngoingIsNoOp(t *testing.T) {
	requireT := require.New(t)
	ctx, keeper := setup(t)

	addressCodec := authcodec.NewBech32Codec(accountBech32Prefix())

	requireT.NoError(RecoverOngoingDistribution(ctx, keeper, addressCodec))

	_, err := keeper.OngoingDistribution.Get(ctx)
	requireT.ErrorIs(err, collections.ErrNotFound)
	_, err = keeper.DistributionDisabled.Get(ctx)
	requireT.ErrorIs(err, collections.ErrNotFound)
}

func TestRecoverOngoingDistribution_IsIdempotent(t *testing.T) {
	requireT := require.New(t)
	ctx, keeper := setup(t)

	addressCodec := authcodec.NewBech32Codec(accountBech32Prefix())
	const distID = uint64(3)

	addr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	score := sdkmath.NewInt(12345)

	seedBrokenState(t, ctx, keeper, distID, []struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}{{addr, score}}, nil)

	requireT.NoError(RecoverOngoingDistribution(ctx, keeper, addressCodec))
	first, err := keeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)

	requireT.NoError(RecoverOngoingDistribution(ctx, keeper, addressCodec))
	second, err := keeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)
	requireT.True(first.Equal(second))
}
