package wdistribution

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAppModule_ConsensusVersion guards the migrations registered in RegisterServices.
// If the distribution module gains a new consensus version, RegisterServices must register its migration too.
func TestAppModule_ConsensusVersion(t *testing.T) {
	require.Equal(t, uint64(3), AppModule{}.ConsensusVersion())
}
