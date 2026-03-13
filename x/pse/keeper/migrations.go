package keeper

// Migrator is a struct for handling in-place store migrations.
type Migrator struct {
}

// NewMigrator returns a new Migrator.
func NewMigrator() Migrator {
	return Migrator{}
}
