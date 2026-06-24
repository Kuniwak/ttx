package main

import (
	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools/ttx-ai-recv/cmd"
)

func main() {
	cli.Run(cmd.MainCommandByArgs)
}
