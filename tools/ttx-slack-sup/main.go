package main

import (
	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools/ttx-slack-sup/cmd"
)

func main() {
	cli.Run(cmd.MainCommandByArgs)
}
