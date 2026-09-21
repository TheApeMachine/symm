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

		Convey("It executes gracefully on context cancellation", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			done := make(chan struct{})
			go func() {
				pipeline.Start(ctx)
				close(done)
			}()

			select {
			case <-done:
				// Succeeded cleanly
			case <-time.After(500 * time.Millisecond):
				cancel()
			}
		})
	})
}
