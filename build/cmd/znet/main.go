package main

import (
	txchainbuild "github.com/tokenize-x/tx-chain/build/tx-chain"
	"github.com/tokenize-x/tx-crust/znet/infra"
	"github.com/tokenize-x/tx-crust/znet/pkg/znet"
)

func main() {
	znet.Main(infra.ConfigFactoryWithTXdUpgrades(txchainbuild.TXdUpgrades()))
}
