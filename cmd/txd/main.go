package main

import (
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client/flags"
	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"

	"github.com/tokenize-x/tx-chain/v7/app"
	"github.com/tokenize-x/tx-chain/v7/cmd/txd/cosmoscmd"
)

const txChainEnvPrefix = "TXD"

func main() {
	network, err := cosmoscmd.PreProcessFlags()
	if err != nil {
		fmt.Printf("Error processing chain id flag, err: %s", err)
		os.Exit(1)
	}
	network.SetSDKConfig()
	app.ChosenNetwork = network

	rootCmd := cosmoscmd.NewRootCmd()
	cosmoscmd.OverwriteDefaultChainIDFlags(rootCmd)
	rootCmd.PersistentFlags().String(flags.FlagChainID, string(app.DefaultChainID), "The network chain ID")
	if err := svrcmd.Execute(rootCmd, txChainEnvPrefix, app.DefaultNodeHome); err != nil {
		//nolint:errcheck // we are already exiting the app so we don't check error.
		fmt.Fprintln(rootCmd.OutOrStderr(), err)
		os.Exit(1)
	}
}
