package main

import (
	selfBuild "github.com/tokenize-x/tx-chain/build"
	selfTools "github.com/tokenize-x/tx-chain/build/tools"
	txchain "github.com/tokenize-x/tx-chain/build/tx-chain"
	"github.com/tokenize-x/tx-crust/build"
	"github.com/tokenize-x/tx-crust/build/tools"
)

func init() {
	tools.AddTools(selfTools.Tools...)
}

func main() {
	build.Main(selfBuild.Commands, txchain.RegisterTXdOSFlag)
}
