package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestCausalVariance(t *testing.T) {
	Convey("Given a causal variance read across two series", t, func() {
		ctx := context.Background()
		client := statistic.CausalVariance_ServerToClient(statistic.NewCausalVariance())
		defer client.Release()

		read := func(scope string, value float64) float64 {
			So(client.Write(ctx, func(params statistic.CausalVariance_write_Params) error {
				params.SetValue(value)
				return params.SetScope(scope)
			}), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			return results.Out()
		}

		read("a", 0)
		read("a", 10)
		So(read("a", 0), ShouldEqual, 50)

		Convey("A new series starts with no spread of its own", func() {
			So(read("b", 5), ShouldEqual, 0)
			So(read("b", 5), ShouldEqual, 0)
		})
	})
}
