package cmd_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestExecute(t *testing.T) {
	Convey("Given the system pipeline definition", t, func() {
		pipeline, err := compiler.CompileFile("../manifest/system.json", nil, compiler.DefaultRepository())
		So(err, ShouldBeNil)
		So(pipeline, ShouldNotBeNil)
		defer pipeline.Release()

		Convey("It executes gracefully on context cancellation", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- pipeline.Start(ctx)
			}()

			select {
			case err := <-done:
				So(err, ShouldBeNil)
			case <-time.After(500 * time.Millisecond):
				t.Fatal("system graph failed to stop after cancellation")
			}
		})
	})
}
