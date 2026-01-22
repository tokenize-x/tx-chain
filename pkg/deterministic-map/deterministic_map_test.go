package deterministicmap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelete(t *testing.T) {
	m := New[string, string]()
	m.Set("a", "b")
	require.Equal(t, 1, m.Len())
	m.Delete("a")
	require.Equal(t, 0, m.Len())
	m.Delete("a") // noop
	require.Equal(t, 0, m.Len())
}
