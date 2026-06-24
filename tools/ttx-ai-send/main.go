package main

import (
	"github.com/Kuniwak/ttx/cli"
	"github.com/Kuniwak/ttx/tools/ttx-ai-send/cmd"
)

func main() {
	cli.Run(cmd.MainCommandByArgs)
}
