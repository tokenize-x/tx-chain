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

var (
	_ extendedMsg = &MsgUpdateExcludedAddresses{}
	_ extendedMsg = &MsgUpdateClearingAccountMappings{}
	_ extendedMsg = &MsgUpdateAllocationSchedule{}
)

// RegisterLegacyAminoCodec registers the amino types and interfaces.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	legacy.RegisterAminoMsg(cdc, &MsgUpdateExcludedAddresses{}, ModuleName+"/MsgUpdateExcludedAddresses")
	legacy.RegisterAminoMsg(cdc, &MsgUpdateClearingAccountMappings{}, ModuleName+"/MsgUpdateClearingAccountMappings")
	legacy.RegisterAminoMsg(cdc, &MsgUpdateAllocationSchedule{}, ModuleName+"/MsgUpdateAllocationSchedule")
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

// ValidateBasic checks that message fields are valid.
func (m *MsgUpdateClearingAccountMappings) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return cosmoserrors.ErrInvalidAddress.Wrapf("invalid authority address: %s", err)
	}

	// Get required accounts
	requiredAccounts := GetNonCommunityClearingAccounts()

	// For governance messages, must provide exact count of all non-Community clearing accounts
	if len(m.Mappings) != len(requiredAccounts) {
		return cosmoserrors.ErrInvalidRequest.Wrapf(
			"invalid non-Community clearing accounts: expected %d mappings, got %d",
			len(requiredAccounts), len(m.Mappings))
	}

	// Validate individual mappings
	if err := validateClearingAccountMappings(m.Mappings); err != nil {
		return err
	}

	// Check all required accounts are provided
	seenAccounts := make(map[string]bool)
	for _, mapping := range m.Mappings {
		seenAccounts[mapping.ClearingAccount] = true
	}
	for _, required := range requiredAccounts {
		if !seenAccounts[required] {
			return cosmoserrors.ErrInvalidRequest.Wrapf("missing required clearing account: %s", required)
		}
	}

	return nil
}

// ValidateBasic checks that message fields are valid.
func (m *MsgUpdateAllocationSchedule) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return cosmoserrors.ErrInvalidAddress.Wrapf("invalid authority address: %s", err)
	}

	// Validate the schedule (includes all clearing account validation)
	return ValidateAllocationSchedule(m.Schedule)
}
