package main

import (
	"github.com/tokenize-x/crust/build"
	"github.com/tokenize-x/crust/build/tools"
	selfBuild "github.com/tokenize-x/tx-chain/build"
	selfTools "github.com/tokenize-x/tx-chain/build/tools"
)

func init() {
	tools.AddTools(selfTools.Tools...)
}

func main() {
	build.Main(selfBuild.Commands)
}
