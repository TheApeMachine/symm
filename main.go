package main

import (
	"github.com/theapemachine/symm/cmd"
)

func main() {
	cmd.Register(cmd.ExperimentalCommand())
	cmd.Execute()
}
