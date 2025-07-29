package cosmoscmd

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/stretchr/testify/require"
)

func TestModifyArgs(t *testing.T) {
	testCases := []struct {
		input    []string
		flag     string
		newVal   string
		expected []string
		hasErr   bool
	}{
		{
			input:    []string{"txd", "--someFlag=1/2"},
			flag:     flags.FlagHome,
			newVal:   "3",
			expected: []string{"txd", "--someFlag=1/2"},
			hasErr:   true,
		},
		{
			input:    []string{"txd", "--someFlag=1/2"},
			flag:     "someFlag",
			newVal:   "3",
			expected: []string{"txd", "--someFlag=1/2/3"},
		},
		{
			input:    []string{"txd", "--home=1/2", "--chain-id=ch1"},
			flag:     flags.FlagHome,
			newVal:   "3",
			expected: []string{"txd", "--home=1/2/3", "--chain-id=ch1"},
		},
		{
			input:    []string{"txd", "--home=1/2"},
			flag:     flags.FlagHome,
			newVal:   "3",
			expected: []string{"txd", "--home=1/2/3"},
		},
		{
			input:    []string{"txd", "--home", "1/2"},
			flag:     flags.FlagHome,
			newVal:   "3",
			expected: []string{"txd", "--home", "1/2/3"},
		},
		{
			input:    []string{"txd", "--home=1/2/"},
			flag:     flags.FlagHome,
			newVal:   "3",
			expected: []string{"txd", "--home=1/2/3"},
		},
		{
			input:    []string{"txd", "--home", "1/2/"},
			flag:     flags.FlagHome,
			newVal:   "3",
			expected: []string{"txd", "--home", "1/2/3"},
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run("", func(t *testing.T) {
			requireT := require.New(t)
			err := appendStringFlag(tc.input, tc.flag, tc.newVal)
			if tc.hasErr {
				requireT.Error(err)
			} else {
				requireT.NoError(err)
			}
			requireT.Equal(tc.expected, tc.input)
		})
	}
}
