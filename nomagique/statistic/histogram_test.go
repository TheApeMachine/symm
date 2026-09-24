package statistic_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestHistogram(t *testing.T) {
	ctx := context.Background()

	Convey("Given a set of values", t, func() {
		client := statistic.Histogram_ServerToClient(statistic.NewHistogram())
		defer client.Release()

		bins := func(values string) ([]struct {
			Lower, Upper float64
			Count        int
		}, bool) {
			So(client.Write(ctx, func(params statistic.Histogram_write_Params) error {
				return params.SetValues([]byte(values))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			if results.Which() != statistic.Binned_Which_out {
				return nil, false
			}

			out, err := results.Out()
			So(err, ShouldBeNil)

			var decoded []struct {
				Lower, Upper float64
				Count        int
			}
			So(json.Unmarshal(out, &decoded), ShouldBeNil)
			return decoded, true
		}

		Convey("Every value lands in one bin whose width comes from the spread", func() {
			decoded, ok := bins(`[1,2,2,3,3,3,4,4,5,9]`)
			So(ok, ShouldBeTrue)
			total := 0

			for _, bin := range decoded {
				So(bin.Upper, ShouldBeGreaterThan, bin.Lower)
				total += bin.Count
			}

			So(total, ShouldEqual, 10)
		})

		Convey("Values with no spread have no distribution to draw", func() {
			_, ok := bins(`[2,2,2,2,2]`)
			So(ok, ShouldBeFalse)
		})
	})
}
