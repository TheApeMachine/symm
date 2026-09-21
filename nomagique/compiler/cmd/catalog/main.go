package main

import (
	"os"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func main() {
	if err := compiler.GenerateCatalogAndRegistry("nomagique"); err != nil {
		errnie.Error(err)
		os.Exit(1)
	}
}
