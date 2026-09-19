package compiler_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestCodegen(t *testing.T) {
	Convey("Given cvd_trade.json definition", t, func() {
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		jsonPath := filepath.Join(repoRoot, "signal", "definitions", "cvd_trade.json")

		generator, err := compiler.NewGenerator(jsonPath)
		So(err, ShouldBeNil)
		So(generator, ShouldNotBeNil)

		src, err := generator.GenerateSource("generated", "CVDTrade")
		So(err, ShouldBeNil)
		So(len(src), ShouldBeGreaterThan, 0)
	})

	Convey("Given hawkes_trade.json definition", t, func() {
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		jsonPath := filepath.Join(repoRoot, "signal", "definitions", "hawkes_trade.json")

		generator, err := compiler.NewGenerator(jsonPath)
		So(err, ShouldBeNil)
		So(generator, ShouldNotBeNil)

		src, err := generator.GenerateSource("generated", "HawkesTrade")
		So(err, ShouldBeNil)
		So(len(src), ShouldBeGreaterThan, 0)
	})

	Convey("Compiling all definitions into generated package", t, func() {
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		defDir := filepath.Join(repoRoot, "signal", "definitions")
		outDir := filepath.Join(repoRoot, "generated")

		err := compiler.CompileAll(defDir, outDir, "generated")
		So(err, ShouldBeNil)
	})
}
