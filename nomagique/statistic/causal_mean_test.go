package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestCausalMean(t *testing.T) {
	Convey("Given a causal mean read across two series", t, func() {
		ctx := context.Background()
		client := statistic.CausalMean_ServerToClient(statistic.NewCausalMean())
		defer client.Release()

		read := func(scope string, value float64) float64 {
			So(client.Write(ctx, func(params statistic.CausalMean_write_Params) error {
				params.SetValue(value)
				return params.SetScope(scope)
			}), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			return results.Out()
		}

		read("a", 2)
		read("a", 4)
		So(read("a", 100), ShouldEqual, 3)

		Convey("A new series is measured against its own history only", func() {
			So(read("b", 10), ShouldEqual, 10)
			So(read("b", 20), ShouldEqual, 10)
		})
	})
}
