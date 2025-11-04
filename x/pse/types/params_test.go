package types

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestDefaultParams(t *testing.T) {
	requireT := require.New(t)

	params := DefaultParams()
	requireT.Empty(params.ExcludedAddresses)
	requireT.NoError(params.ValidateBasic())
}

func TestParamsValidation(t *testing.T) {
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name      string
		params    Params
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_empty_excluded_addresses",
			params: Params{
				ExcludedAddresses: []string{},
			},
			expectErr: false,
		},
		{
			name: "valid_one_excluded_address",
			params: Params{
				ExcludedAddresses: []string{addr1},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_excluded_addresses",
			params: Params{
				ExcludedAddresses: []string{addr1, addr2, addr3},
			},
			expectErr: false,
		},
		{
			name: "invalid_malformed_address",
			params: Params{
				ExcludedAddresses: []string{"invalid-address"},
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_duplicate_address",
			params: Params{
				ExcludedAddresses: []string{addr1, addr2, addr1},
			},
			expectErr: true,
			errMsg:    "duplicate address",
		},
		{
			name: "invalid_empty_string_in_list",
			params: Params{
				ExcludedAddresses: []string{addr1, ""},
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_wrong_prefix",
			params: Params{
				ExcludedAddresses: []string{addr1, "cosmos1invalidprefix"},
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_duplicate_at_end",
			params: Params{
				ExcludedAddresses: []string{addr1, addr2, addr3, addr1},
			},
			expectErr: true,
			errMsg:    "duplicate address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			err := tc.params.ValidateBasic()
			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
			}
		})
	}
}
