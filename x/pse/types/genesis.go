package types

// DefaultGenesisState returns genesis state with default values.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{}
}

// Validate validates genesis parameters.
func (m *GenesisState) Validate() error {
	return nil
}
