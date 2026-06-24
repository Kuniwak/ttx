package main

import (
	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools/ttx-human/cmd"
)

func main() {
	cli.Run(cmd.MainCommandByArgs)
}
