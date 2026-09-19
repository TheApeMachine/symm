package main

import (
	"os"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/catalog/scan"
)

func main() {
	schemas, err := scan.Tree("nomagique")
	if err != nil {
		errnie.Error(err)
		os.Exit(1)
	}

	encoded, err := sonic.MarshalIndent(schemas, "", "\t")
	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "catalog: encode primitives", err))
		os.Exit(1)
	}

	if err := os.WriteFile(
		"nomagique/catalog/primitives.json", append(encoded, '\n'), 0644,
	); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "catalog: write primitives.json", err))
		os.Exit(1)
	}

	if err := scan.GenerateRegistry(schemas); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "catalog: generate registry", err))
		os.Exit(1)
	}
}
