package v6patch1testnet

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/samber/lo"

	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// DefaultParams returns the default PSE params for the given chain ID.
//
//nolint:funlen // large switch with chain-specific mapping literals
func DefaultParams(chainID string) (psetypes.Params, error) {
	// Create mappings for all non-Community clearing accounts
	// Each starts with a single default recipient (can be modified via governance)
	var mappings []psetypes.ClearingAccountMapping
	var otherFoundationAddresses []string

	switch chainID {
	case string(constant.ChainIDMain):
		mappings = []psetypes.ClearingAccountMapping{
			{
				ClearingAccount: psetypes.ClearingAccountFoundation,
				RecipientAddresses: []string{
					"core142498n8sya3k3s5jftp7dujuqfw3ag4tpzc2ve45ykpwx6zmng8skcw5nw",
					"core1ys0dhh6x5s55h2g37zrnc7kh630jfq5p77as8pwyn60ax9zzqh9qvpwc0e",
					"core1wgjpjh42cr7t5sp5hgty4yrzww496a6yaznc9u4wsv9ac3xccu8smqaann",
					"core1rkml5878l2daw3a7xvg48wqecnh9u9dn2dtl8g57rsctq5pnc00sl0nwak",
					"core17l6djqrztw0ux668vkw7ff7d2602jvml52w9fyrvryusp7djnhfq7sg29r",
					"core10ezj2lmcj3flaacqwrzv278aled0pen8cnx257sggeng2fdel53q5gtudn",
					"core1wfse3z8akyw3pmn8x0htzq6l5wwfgqmc2jgnhxtzm96h4ywhhr0q63uvwl",
					"core10w37pqels7ya404xdlfkc9vdfemejmc0e6hjlerknys3xjj9xnasuk9uy2",
					"core13cwsdsetcrhcyd3jeed0mgteg35qaju0q5s0u0drfylagahygwwsj2eanz",
					"core1jc4mtk0g8ulmvhwmpfy5rrj7rwn85ual4p3w0tlwnp2rsauvf5eq58zdmw",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountAlliance,
				RecipientAddresses: []string{
					"core1cfey705ssf6ysclm9u47mvcgr5l6q6q86lk5dtq4jwdu6yjce6ds2tgy6j",
					"core15629hwdy7rd7satqzffn4f80ftg2sln982xvwcalppg36td7jvuq3pqevw",
					"core15lch5glk7deu9tk8wrcfcup4tdpz2l8zhhqn4r2zzsr46dfv849qetkah4",
					"core19rrgcsw8gu8c3rthucqnf6nyyg6q9pq79tt60pvahfsnfu4p5hrsuqajru",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountPartnership,
				RecipientAddresses: []string{
					"core12s5tahy3850k3r3080en0pwhuk4l3my5l2cl8vxrsg6kx48de24q7ygamd",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountInvestors,
				RecipientAddresses: []string{
					"core1mqevjln5hxv3qgd3c4m5zjeeand5hkc7r33ty82fjukw9shxjh6sr0zafz",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountTeam,
				RecipientAddresses: []string{
					"core12xyww2vucfufyzknvyameh5v25cn6gxzzagwgpzhwdq8v35zdmgqd6t6c7",
				},
			},
		}
		otherFoundationAddresses = []string{
			"core13xmyzhvl02xpz0pu8v9mqalsvpyy7wvs9q5f90",
			"core14g6wpzdx8g9txvxxu3fl7fplal9y5ztx34ac5p",
			"core1zn2ns3ls68jlsv5dgkuz0rxsxt5fhk7n9cfl23",
			"core1p4gsfkmqm0uxua65phteqwnmu39fwjvtspfkcj",
			"core1rddqzjzy4f5frxkhds3sux0m03encqtla3ayu9",
			"core1qe7xz56v5sh4mr0vfq8qycnvjudgrslmjt0n3m",
			"core17epxygqaytz5l63f0au04058kt4w72w6pkh0as",
		}
	case string(constant.ChainIDTest):
		mappings = []psetypes.ClearingAccountMapping{
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
		otherFoundationAddresses = []string{
			"testcore1efkcsd94u0vrx8rgq9cktjgq7fgwrjap3qu289",
			"testcore18nfwg708vu74e6mrcu6yjdzcdq5608rmvavt05",
			"testcore1qrqhjrc2jl36l4vuvhvjlt6kg6d0xqazzlxek7",
			"testcore12guwnjehw06c9r40knd0js5dn6g924p7xxg48h",
		}
	case string(constant.ChainIDDev):
		mappings = []psetypes.ClearingAccountMapping{
			{
				ClearingAccount: psetypes.ClearingAccountFoundation,
				RecipientAddresses: []string{
					"devcore17cak5uy6k70l0hqqr3zrkrr960whz6jaqyey0d",
					"devcore1an4p6dscn9r6uq3exsmpus4k0k249quq2n8hlw",
					"devcore12xl22gjn33gpgtt3vnvtgk4lxveyeuyyj9hk9y",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountAlliance,
				RecipientAddresses: []string{
					"devcore1jeq2gxe3kecsmxaevvea6jenzs7wzc38v746pv",
					"devcore1e4taqtkgj34g5wgs6hjjdgm2g4ydzgghx5vzka",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountPartnership,
				RecipientAddresses: []string{
					"devcore1vwsnreaczvgarnchqj0j7sdwd3276japl8rkug",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountInvestors,
				RecipientAddresses: []string{
					"devcore1fvzdawu3m6x39sn72k9h9ql8g3paevn8h0ndrc",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountTeam,
				RecipientAddresses: []string{
					"devcore17hzjn0smfn98mk25mcd7s64wkztn9j32x3ulvw",
				},
			},
		}
		otherFoundationAddresses = []string{
			"devcore1ma6a84s25n9q2f3wlsdwg22a84qknn2fggtrqn",
			"devcore177xv6sted8f2e8z5elz84ypegr47n2d9hs7k2e",
			"devcore17we2jgjyxexcz8rg29dn622axt7s9l263fl0zt",
		}
	default:
		return psetypes.Params{}, errorsmod.Wrapf(psetypes.ErrInvalidInput, "unknown chain id: %s", chainID)
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

// V6ParamsPatch sets the PSE module params to testnet defaults.
func V6ParamsPatch(ctx context.Context, pseKeeper pskeeper.Keeper) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := DefaultParams(sdkCtx.ChainID())
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "failed to get default PSE params: %v", err)
	}
	return pseKeeper.SetParams(ctx, params)
}
