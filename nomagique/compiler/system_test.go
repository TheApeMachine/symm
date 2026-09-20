package compiler_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/definitions"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestSystemOrchestration(t *testing.T) {
	Convey("Given the master system orchestration JSON", t, func() {
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		systemPath := filepath.Join(repoRoot, "signal", "definitions", "system.json")

		builder, err := compiler.NewBuilder(systemPath)
		So(err, ShouldBeNil)
		So(builder, ShouldNotBeNil)

		pipeline, err := builder.Compose(definitions.Default())
		So(err, ShouldBeNil)
		So(pipeline, ShouldNotBeNil)
	})
}
