package compiler_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestBuilderCompose(t *testing.T) {
	Convey("Given signal definitions", t, func() {
		repo := compiler.DefaultRepository()
		ids, err := repo.List()
		So(err, ShouldBeNil)
		So(len(ids), ShouldBeGreaterThan, 0)

		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

		for _, id := range ids {
			Convey("Compiling "+id, func() {
				jsonPath := filepath.Join(repoRoot, "manifest", id+".json")
				builder, err := compiler.NewBuilder(jsonPath)
				So(err, ShouldBeNil)
				So(builder, ShouldNotBeNil)

				pipeline, err := builder.Compose(repo)
				if err != nil {
					So(err.Error(), ShouldNotBeEmpty)
					return
				}
				So(pipeline, ShouldNotBeNil)
			})
		}
	})
}
