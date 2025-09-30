package main

import (
	"github.com/tokenize-x/crust/znet/infra"
	"github.com/tokenize-x/crust/znet/pkg/znet"
	txchainbuild "github.com/tokenize-x/tx-chain/build/tx-chain"
)

func main() {
	znet.Main(infra.ConfigFactoryWithTXdUpgrades(txchainbuild.TXdUpgrades()))
}
