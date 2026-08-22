package main

import (
	"github.com/theapemachine/symm/backtest/driver"
	"github.com/theapemachine/symm/cmd"
)

func main() {
	cmd.Register(driver.Command())
	cmd.Register(cmd.ExperimentalCommand())
	cmd.Execute()
}
