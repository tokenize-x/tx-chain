package cli

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// GetQueryCmd returns the parent command for all CLI query commands. The
// provided clientCtx should have, at a minimum, a verifier, Tendermint RPC client,
// and marshaler set.
func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Querying commands for the pse module",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(CmdQueryParams())
	cmd.AddCommand(CmdQueryScore())
	cmd.AddCommand(CmdQueryScheduledDistributions())
	cmd.AddCommand(CmdQueryClearingAccountBalances())

	return cmd
}

// CmdQueryParams implements a command to fetch dex parameters.
func CmdQueryParams() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "params",
		Short: fmt.Sprintf("Query the current %s parameters", types.ModuleName),
		Args:  cobra.NoArgs,
		Long: strings.TrimSpace(
			fmt.Sprintf(`Query parameters for the %s module:

Example:
$ %[1]s query %s params
`,
				types.ModuleName, version.AppName, types.ModuleName,
			),
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			queryClient := types.NewQueryClient(clientCtx)

			params := &types.QueryParamsRequest{}
			res, err := queryClient.Params(cmd.Context(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// CmdQueryScore implements a command to fetch pse score of an address.
func CmdQueryScore() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "score [address]",
		Short: "Query the score of an address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			queryClient := types.NewQueryClient(clientCtx)
			scoreRequest := &types.QueryScoreRequest{
				Address: args[0],
			}
			res, err := queryClient.Score(cmd.Context(), scoreRequest)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// CmdQueryScheduledDistributions implements a command to fetch all future scheduled distributions.
func CmdQueryScheduledDistributions() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduled-distributions",
		Short: "Query all future scheduled distributions",
		Args:  cobra.NoArgs,
		Long: strings.TrimSpace(
			fmt.Sprintf(`Query all future scheduled distributions for the %s module:

Example:
$ %[1]s query %s scheduled-distributions
`,
				types.ModuleName, version.AppName, types.ModuleName,
			),
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			queryClient := types.NewQueryClient(clientCtx)

			params := &types.QueryScheduledDistributionsRequest{}
			res, err := queryClient.ScheduledDistributions(cmd.Context(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// CmdQueryClearingAccountBalances implements a command to fetch the current balances of all PSE clearing accounts.
func CmdQueryClearingAccountBalances() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clearing-account-balances",
		Short: "Query the current balances of all PSE clearing accounts",
		Args:  cobra.NoArgs,
		Long: strings.TrimSpace(
			fmt.Sprintf(`Query the current balances of all PSE clearing accounts:

Example:
$ %[1]s query %s clearing-account-balances
`,
				types.ModuleName, version.AppName, types.ModuleName,
			),
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			queryClient := types.NewQueryClient(clientCtx)

			params := &types.QueryClearingAccountBalancesRequest{}
			res, err := queryClient.ClearingAccountBalances(cmd.Context(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}
