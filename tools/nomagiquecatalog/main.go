/*
Command nomagiquecatalog regenerates the description of nomagique's primitives
that the pipeline editor draws its palette from.

It has no runtime relationship to the trading system: it reads Go source and
writes one JSON document. Run it after adding, removing, or re-signaturing a
nomagique constructor — `nomagique/catalog`'s test fails until it is run, so
the committed document can never quietly describe a different library than the
one beside it.

	go run ./tools/nomagiquecatalog
*/
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

	// Indented and newline-terminated: the document is committed, so its diffs
	// are read by people.
	encoded, err := sonic.MarshalIndent(schemas, "", "\t")

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "catalog: encode primitives", err))
		os.Exit(1)
	}

	if err := os.WriteFile(
		"nomagique/catalog/primitives.json", append(encoded, '\n'), 0o644,
	); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "catalog: write primitives.json", err))
		os.Exit(1)
	}
}
