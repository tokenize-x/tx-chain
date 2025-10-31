package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/legacy"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
)

type extendedMsg interface {
	sdk.Msg
	sdk.HasValidateBasic
}

var _ extendedMsg = &MsgUpdateExcludedAddresses{}

// RegisterLegacyAminoCodec registers the amino types and interfaces.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	legacy.RegisterAminoMsg(cdc, &MsgUpdateExcludedAddresses{}, ModuleName+"/MsgUpdateExcludedAddresses")
}

// ValidateBasic checks that message fields are valid.
func (m *MsgUpdateExcludedAddresses) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return cosmoserrors.ErrInvalidAddress.Wrapf("invalid authority address: %s", err)
	}

	// At least one operation (add or remove) must be specified
	if len(m.AddressesToAdd) == 0 && len(m.AddressesToRemove) == 0 {
		return cosmoserrors.ErrInvalidRequest.Wrap("must specify at least one address to add or remove")
	}

	// Validate all addresses to add
	for _, addr := range m.AddressesToAdd {
		if _, err := sdk.AccAddressFromBech32(addr); err != nil {
			return cosmoserrors.ErrInvalidAddress.Wrapf("invalid address to add: %s", err)
		}
	}

	// Validate all addresses to remove
	for _, addr := range m.AddressesToRemove {
		if _, err := sdk.AccAddressFromBech32(addr); err != nil {
			return cosmoserrors.ErrInvalidAddress.Wrapf("invalid address to remove: %s", err)
		}
	}

	// Check for duplicates in add list
	addSet := make(map[string]bool)
	for _, addr := range m.AddressesToAdd {
		if addSet[addr] {
			return cosmoserrors.ErrInvalidRequest.Wrapf("duplicate address in add list: %s", addr)
		}
		addSet[addr] = true
	}

	// Check for duplicates in remove list
	removeSet := make(map[string]bool)
	for _, addr := range m.AddressesToRemove {
		if removeSet[addr] {
			return cosmoserrors.ErrInvalidRequest.Wrapf("duplicate address in remove list: %s", addr)
		}
		removeSet[addr] = true
	}

	// Check for addresses that are in both lists
	for _, addr := range m.AddressesToAdd {
		if removeSet[addr] {
			return cosmoserrors.ErrInvalidRequest.Wrapf("address %s cannot be in both add and remove lists", addr)
		}
	}

	return nil
}
