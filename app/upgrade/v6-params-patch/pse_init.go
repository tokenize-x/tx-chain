package v6paramspatch

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	"github.com/samber/lo"

	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// DefaultTestnetParams returns the default PSE params for testnet.
func DefaultTestnetParams() (psetypes.Params, error) {
	mappings := []psetypes.ClearingAccountMapping{
		{
			ClearingAccount: psetypes.ClearingAccountFoundation,
			RecipientAddresses: []string{
				"testcore17rzcx6c37ypp8m6hrl6pyhhl3mfp2s5d6xhyyl23vsj3laclhpxqx89alr",
				"testcore19kswr87wtx95gphrmkr785595untfmf9fd4dag4chthl5fxnkuhsc3v7gk",
				"testcore16vth8ad0anjqpqqmwpfzc09c3w2tj4492vz6zzwr0xk9st6ca0tsm3nyv4",
				"testcore1hmgca4jxfuxmg8lja9sdet307cldcpm4f6ttacurx8d4d03jz2aq5jgzwm",
				"testcore1c67vg6kueqn5wd78vu0drfqtq7rurhulngyulc9qc0glk9l36vsq4v8h44",
				"testcore1590eujlxwl7qsllu77xeu9v8ryuupkn6s0q5tlyp2e8ea6wa39tqpjy9sx",
				"testcore1xc505dp7agzg7rnzzfzmllmqckw32et0rdnpwck3cplylgplj9hqwnnnvp",
				"testcore13qrxcrsj69kztezt8pepmjeemen5tzxyx3wkg8mtllg2sexwgp2qs9rg2g",
				"testcore1kxsc00mvmhx4mqklzhzze3nr56d0ejclpcda3nf8e6cqcap9mvzq2v6gzk",
				"testcore120xxdn7hydfc8j2aak902zwlmuh9px465ft5jraj7l6qy5ksws4se0ucz7",
			},
		},
		{
			ClearingAccount: psetypes.ClearingAccountAlliance,
			RecipientAddresses: []string{
				"testcore1csd2z5ycyvfumnjdr7qsgw2r0y9uc7nsk4a4596ej275rg0lzwrqr5g4yy",
				"testcore13egmenzagvcfnldcupxg5zfx5rgjrq44ugzewugku4l7e4jtmvns28sja8",
			},
		},
		{
			ClearingAccount: psetypes.ClearingAccountPartnership,
			RecipientAddresses: []string{
				"testcore1ludesr02ls9gjv4ufzg9kwypdn8uxvxmk65hqznxnf46hkfcsffqx4ktqv",
			},
		},
		{
			ClearingAccount: psetypes.ClearingAccountInvestors,
			RecipientAddresses: []string{
				"testcore16hu0xamesjwemrw4u3tpp23dkv3y2htgxvd2k942v3ekus2gsj5qsenwy3",
			},
		},
		{
			ClearingAccount: psetypes.ClearingAccountTeam,
			RecipientAddresses: []string{
				"testcore1lurev2l3g5pecey8lgywxw8wqvs4zupxqvmw4twmr9s8jlll6pgscmsu38",
			},
		},
	}
	otherFoundationAddresses := []string{
		"testcore1efkcsd94u0vrx8rgq9cktjgq7fgwrjap3qu289",
		"testcore18nfwg708vu74e6mrcu6yjdzcdq5608rmvavt05",
		"testcore1qrqhjrc2jl36l4vuvhvjlt6kg6d0xqazzlxek7",
		"testcore12guwnjehw06c9r40knd0js5dn6g924p7xxg48h",
	}

	var allExcludedAddresses []string
	for _, mapping := range mappings {
		allExcludedAddresses = append(allExcludedAddresses, mapping.RecipientAddresses...)
	}
	allExcludedAddresses = lo.Uniq(append(allExcludedAddresses, otherFoundationAddresses...))

	return psetypes.Params{
		ExcludedAddresses:       allExcludedAddresses,
		ClearingAccountMappings: mappings,
	}, nil
}

// TestnetV6Patch sets the PSE module params to testnet defaults.
func TestnetV6Patch(ctx context.Context, pseKeeper pskeeper.Keeper) error {
	params, err := DefaultTestnetParams()
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "failed to get default PSE params: %v", err)
	}
	return pseKeeper.SetParams(ctx, params)
}
